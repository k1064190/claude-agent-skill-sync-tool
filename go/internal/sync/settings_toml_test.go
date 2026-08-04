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

func TestMergeTomlAddsRootStopHookWithoutChangingHookTrust(t *testing.T) {
	live := `notify = ["codex-notify"]

[hooks.state."/home/cwh/.codex/hooks.json:stop:0:0"]
enabled = true
trusted_hash = "sha256:abc"
`
	fragment := `notify = []
hooks.Stop = [{ hooks = [{ type = "command", command = "$HOME/.local/bin/codex-notify", timeout = 10 }] }]
`
	merged, changed, err := MergeTomlSettingsContent([]byte(live), []byte(fragment))
	if err != nil {
		t.Fatalf("MergeTomlSettingsContent: %v", err)
	}
	if !changed {
		t.Fatal("changed = false; want true")
	}
	got := string(merged)
	for _, must := range []string{
		`notify = []`,
		`hooks.Stop = [{ hooks = [{ type = "command", command = "$HOME/.local/bin/codex-notify", timeout = 10 }] }]`,
		`[hooks.state."/home/cwh/.codex/hooks.json:stop:0:0"]`,
		`trusted_hash = "sha256:abc"`,
	} {
		if !strings.Contains(got, must) {
			t.Errorf("merged config lost %q", must)
		}
	}
	if strings.Contains(got, `notify = ["codex-notify"]`) {
		t.Error("legacy notifier registration survived")
	}
	if strings.Index(got, "hooks.Stop") > strings.Index(got, "[hooks.state.") {
		t.Error("root Stop hook was placed inside the preserved hook-trust table")
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

func TestRemoveTomlSettingsKeysRemovesTopLevelAndTableKeys(t *testing.T) {
	live := []byte(`service_tier = "fast"
model = "gpt-5.6-sol"

[features]
fast_mode = true
plugins = true

[projects."/home/cwh"]
trust_level = "trusted"
service_tier = "project-local"
`)

	got, changed, err := RemoveTomlSettingsKeys(live, []string{"service_tier", "features.fast_mode"})
	if err != nil {
		t.Fatalf("RemoveTomlSettingsKeys: %v", err)
	}
	if !changed {
		t.Fatal("changed = false; want true")
	}
	text := string(got)
	for _, removed := range []string{`service_tier = "fast"`, `fast_mode = true`} {
		if strings.Contains(text, removed) {
			t.Errorf("removed setting survived: %q", removed)
		}
	}
	for _, preserved := range []string{`model = "gpt-5.6-sol"`, `plugins = true`, `[projects."/home/cwh"]`, `trust_level = "trusted"`, `service_tier = "project-local"`} {
		if !strings.Contains(text, preserved) {
			t.Errorf("unmanaged setting was lost: %q", preserved)
		}
	}
}

func TestRemoveTomlSettingsKeysRefusesMultilineTarget(t *testing.T) {
	live := []byte("[features]\nfast_mode = [\n  true,\n]\n")
	if _, _, err := RemoveTomlSettingsKeys(live, []string{"features.fast_mode"}); err == nil {
		t.Fatal("expected multiline target error")
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

// P1 #1 (review): a top-level multi-line string whose continuation line starts
// with '[' must NOT be mistaken for a table header. The line-based surgery cannot
// safely edit around a top-level multi-line string, so it refuses rather than
// corrupt the file.
func TestMergeTomlRefusesTopLevelMultilineString(t *testing.T) {
	live := `description = """
[projects]
trust = yes
"""
model = "gpt-5"

[real_table]
x = 1
`
	if _, _, err := MergeTomlSettingsContent([]byte(live), []byte(`sandbox_mode = "workspace-write"`)); err == nil {
		t.Error("expected a refusal for a top-level multi-line string, got nil (risk of corruption)")
	}
}

// A multi-line string inside a [table] (not the top-level region) is fine — it
// is in the preserved-verbatim tables section and never parsed.
func TestMergeTomlAllowsMultilineInsideTable(t *testing.T) {
	live := `model = "gpt-5"

[some_table]
note = """
[not a table]
"""
`
	merged, _, err := MergeTomlSettingsContent([]byte(live), []byte(`sandbox_mode = "workspace-write"`))
	if err != nil {
		t.Fatalf("multi-line string inside a table should be preserved, got error: %v", err)
	}
	if !strings.Contains(string(merged), "[not a table]") {
		t.Error("multi-line string content inside a table was not preserved")
	}
	// sandbox_mode must be appended before the first table.
	if strings.Index(string(merged), "sandbox_mode") > strings.Index(string(merged), "[some_table]") {
		t.Error("owned key placed after a table header")
	}
}

// P1 #2 (review): an inline comment on a fragment value must be stripped, not
// carried into the value.
func TestMergeTomlStripsFragmentInlineComment(t *testing.T) {
	merged, _, err := MergeTomlSettingsContent(nil, []byte(`sandbox_mode = "workspace-write"  # new mode`))
	if err != nil {
		t.Fatalf("MergeTomlSettingsContent: %v", err)
	}
	got := string(merged)
	if !strings.Contains(got, `sandbox_mode = "workspace-write"`) {
		t.Errorf("value not written correctly: %q", got)
	}
	if strings.Contains(got, "# new mode") {
		t.Error("fragment inline comment leaked into the written value")
	}
}

// A '#' inside a quoted value is part of the value, not a comment.
func TestMergeTomlKeepsHashInsideQuotes(t *testing.T) {
	merged, _, err := MergeTomlSettingsContent(nil, []byte(`token = "abc#123"`))
	if err != nil {
		t.Fatalf("MergeTomlSettingsContent: %v", err)
	}
	if !strings.Contains(string(merged), `token = "abc#123"`) {
		t.Errorf("hash inside quotes was wrongly stripped: %q", merged)
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
