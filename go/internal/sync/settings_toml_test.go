package sync

import (
	"strings"
	"testing"
)

// A realistic Codex config.toml: three top-level scalars, then machine-specific
// tables that must survive byte-for-byte.
const codexLive = `model = "gpt-5.6-sol"
model_reasoning_effort = "xhigh"
personality = "pragmatic"

[projects."/home/cwh"]
trust_level = "trusted"

[hooks.state."/home/cwh/.codex/hooks.json:pre_tool_use:0:0"]
enabled = true
trusted_hash = "sha256:abc"
`

func TestMergeTomlOwnsTopLevelKeys(t *testing.T) {
	fragment := `sandbox_mode = "workspace-write"
approval_policy = "on-request"
`
	merged, changed, err := MergeTomlSettingsContent([]byte(codexLive), []byte(fragment))
	if err != nil {
		t.Fatalf("MergeTomlSettingsContent: %v", err)
	}
	if !changed {
		t.Fatal("changed = false; want true")
	}
	got := string(merged)

	// Owned keys were appended (they were absent).
	if !strings.Contains(got, `sandbox_mode = "workspace-write"`) {
		t.Error("sandbox_mode not injected")
	}
	if !strings.Contains(got, `approval_policy = "on-request"`) {
		t.Error("approval_policy not injected")
	}

	// Machine tables preserved verbatim, in order, unchanged.
	for _, must := range []string{
		`[projects."/home/cwh"]`,
		`trust_level = "trusted"`,
		`[hooks.state."/home/cwh/.codex/hooks.json:pre_tool_use:0:0"]`,
		`trusted_hash = "sha256:abc"`,
	} {
		if !strings.Contains(got, must) {
			t.Errorf("machine table content lost: %q", must)
		}
	}

	// The new keys must sit BEFORE the first table (top-level keys only live there).
	if strings.Index(got, `sandbox_mode`) > strings.Index(got, "[projects.") {
		t.Error("sandbox_mode was placed after a table header; TOML would misassign it")
	}

	// Unowned top-level scalars untouched.
	if !strings.Contains(got, `model = "gpt-5.6-sol"`) {
		t.Error("unowned key model was lost")
	}
}

func TestMergeTomlReplacesExistingKey(t *testing.T) {
	live := `model = "gpt-5.6-sol"
sandbox_mode = "read-only"

[projects."/home/cwh"]
trust_level = "trusted"
`
	fragment := `sandbox_mode = "workspace-write"
`
	merged, changed, err := MergeTomlSettingsContent([]byte(live), []byte(fragment))
	if err != nil {
		t.Fatalf("MergeTomlSettingsContent: %v", err)
	}
	if !changed {
		t.Fatal("changed = false; want true")
	}
	got := string(merged)
	if strings.Contains(got, `sandbox_mode = "read-only"`) {
		t.Error("old sandbox_mode value survived; owned keys must be replaced")
	}
	if !strings.Contains(got, `sandbox_mode = "workspace-write"`) {
		t.Error("new sandbox_mode value not written")
	}
	if strings.Count(got, "sandbox_mode") != 1 {
		t.Errorf("sandbox_mode appears %d times; want exactly 1", strings.Count(got, "sandbox_mode"))
	}
}

func TestMergeTomlNoChangeWhenApplied(t *testing.T) {
	live := `model = "gpt-5.6-sol"
sandbox_mode = "workspace-write"

[projects."/home/cwh"]
trust_level = "trusted"
`
	fragment := `sandbox_mode = "workspace-write"
`
	merged, changed, err := MergeTomlSettingsContent([]byte(live), []byte(fragment))
	if err != nil {
		t.Fatalf("MergeTomlSettingsContent: %v", err)
	}
	if changed {
		t.Errorf("changed = true; want false when already applied.\nmerged:\n%s", merged)
	}
}

func TestMergeTomlEmptyLive(t *testing.T) {
	fragment := `sandbox_mode = "workspace-write"
approval_policy = "on-request"
`
	merged, changed, err := MergeTomlSettingsContent(nil, []byte(fragment))
	if err != nil {
		t.Fatalf("MergeTomlSettingsContent: %v", err)
	}
	if !changed {
		t.Fatal("changed = false; want true")
	}
	if !strings.Contains(string(merged), `sandbox_mode = "workspace-write"`) {
		t.Error("fragment not written to empty live file")
	}
}

// A fragment must declare only top-level keys. A table in the fragment would
// mean claiming ownership of a machine-specific section (projects, hooks.state),
// which is never intended and must be rejected loudly.
func TestMergeTomlRejectsTableInFragment(t *testing.T) {
	fragment := `sandbox_mode = "workspace-write"

[projects."/x"]
trust_level = "trusted"
`
	if _, _, err := MergeTomlSettingsContent([]byte(codexLive), []byte(fragment)); err == nil {
		t.Error("expected an error for a fragment containing a table, got nil")
	}
}

// A comment attached to an owned key, and preamble comments in general, must be
// preserved (we only rewrite the assignment line).
func TestMergeTomlPreservesComments(t *testing.T) {
	live := `# top of file
model = "gpt-5.6-sol"  # the model

[tui]
theme = "dark"
`
	fragment := `sandbox_mode = "workspace-write"
`
	merged, _, err := MergeTomlSettingsContent([]byte(live), []byte(fragment))
	if err != nil {
		t.Fatalf("MergeTomlSettingsContent: %v", err)
	}
	got := string(merged)
	if !strings.Contains(got, "# top of file") || !strings.Contains(got, `model = "gpt-5.6-sol"  # the model`) {
		t.Error("preamble comments were not preserved")
	}
}
