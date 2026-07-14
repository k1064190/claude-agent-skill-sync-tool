package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/k1064190/claude-agent-skill-sync-tool/go/internal/config"
)

func TestSettingsTargetName(t *testing.T) {
	if got := SettingsTargetName(config.PlatformClaude); got != "settings.json" {
		t.Errorf("SettingsTargetName(Claude) = %q; want settings.json", got)
	}
	// Platforms without a supported settings file yield "".
	for _, p := range []config.Platform{config.PlatformCodex, config.PlatformAntigravity, config.PlatformOpencode} {
		if got := SettingsTargetName(p); got != "" {
			t.Errorf("SettingsTargetName(%v) = %q; want \"\"", p, got)
		}
	}
}

// TestMergeSettingsContent covers the ownership semantics: top-level keys
// present in the fragment replace the live value wholesale (so removing an
// entry from a fragment object propagates), and keys absent from the fragment
// are preserved untouched (so machine-local settings never churn).
func TestMergeSettingsContent(t *testing.T) {
	live := []byte(`{
  "theme": "light",
  "effortLevel": "xhigh",
  "enabledPlugins": {"a@m": true, "stale@m": true}
}`)
	fragment := []byte(`{
  "enabledPlugins": {"a@m": true, "b@m": true},
  "extraKnownMarketplaces": {"m": {"repo": "x/y"}}
}`)

	merged, changed, err := MergeSettingsContent(live, fragment)
	if err != nil {
		t.Fatalf("MergeSettingsContent: %v", err)
	}
	if !changed {
		t.Error("changed = false; want true")
	}

	var got map[string]any
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatalf("merged output is not valid JSON: %v", err)
	}

	// Unowned keys preserved.
	if got["theme"] != "light" {
		t.Errorf("theme = %v; want light (unowned keys must be preserved)", got["theme"])
	}
	if got["effortLevel"] != "xhigh" {
		t.Errorf("effortLevel = %v; want xhigh", got["effortLevel"])
	}

	// Owned key replaced wholesale: "stale@m" must be gone, "b@m" added.
	plugins, ok := got["enabledPlugins"].(map[string]any)
	if !ok {
		t.Fatalf("enabledPlugins = %v; want object", got["enabledPlugins"])
	}
	if _, exists := plugins["stale@m"]; exists {
		t.Error("stale@m still present; owned top-level keys must be replaced, not deep-merged")
	}
	if plugins["a@m"] != true || plugins["b@m"] != true {
		t.Errorf("enabledPlugins = %v; want a@m and b@m true", plugins)
	}

	// New owned key added.
	if _, exists := got["extraKnownMarketplaces"]; !exists {
		t.Error("extraKnownMarketplaces missing")
	}
}

// TestMergeSettingsContentNoChange verifies an already-applied fragment reports
// changed=false, so --refresh stays a no-op and writes no backup.
func TestMergeSettingsContentNoChange(t *testing.T) {
	live := []byte(`{"theme": "light", "enabledPlugins": {"a@m": true}}`)
	fragment := []byte(`{"enabledPlugins": {"a@m": true}}`)

	_, changed, err := MergeSettingsContent(live, fragment)
	if err != nil {
		t.Fatalf("MergeSettingsContent: %v", err)
	}
	if changed {
		t.Error("changed = true; want false when the fragment is already applied")
	}
}

// TestMergeSettingsContentEmptyLive covers a settings file that does not exist
// yet (empty bytes) — the fragment becomes the whole file.
func TestMergeSettingsContentEmptyLive(t *testing.T) {
	merged, changed, err := MergeSettingsContent(nil, []byte(`{"enabledPlugins": {"a@m": true}}`))
	if err != nil {
		t.Fatalf("MergeSettingsContent: %v", err)
	}
	if !changed {
		t.Error("changed = false; want true")
	}
	var got map[string]any
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, exists := got["enabledPlugins"]; !exists {
		t.Error("enabledPlugins missing")
	}
}

// TestMergeSettingsContentMalformedLive verifies a corrupt live file is a hard
// error — never silently overwritten.
func TestMergeSettingsContentMalformedLive(t *testing.T) {
	if _, _, err := MergeSettingsContent([]byte(`{not json`), []byte(`{"a": 1}`)); err == nil {
		t.Error("expected an error for malformed live settings, got nil")
	}
}

// TestApplySettingsPreservesModeAndBacksUpSecurely verifies that the live
// file's permission bits survive the rewrite (the tool owns some keys, not the
// file's mode) and that the backup is not world-readable.
func TestApplySettingsPreservesModeAndBacksUpSecurely(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "settings")
	destDir := filepath.Join(tempDir, ".claude")
	for _, d := range []string{srcDir, destDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(srcDir, "claude.json"),
		[]byte(`{"enabledPlugins": {"a@m": true}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	destPath := filepath.Join(destDir, "settings.json")
	if err := os.WriteFile(destPath, []byte(`{"theme": "light"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := ApplySettings(srcDir, destDir, config.PlatformClaude); err != nil {
		t.Fatalf("ApplySettings: %v", err)
	}

	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("settings.json mode = %#o; want 0600 preserved", got)
	}

	bak, err := os.Stat(destPath + ".bak")
	if err != nil {
		t.Fatalf("backup not created: %v", err)
	}
	if got := bak.Mode().Perm(); got != 0o600 {
		t.Errorf("backup mode = %#o; want 0600", got)
	}
}

// TestApplySettingsLeavesNoTempFiles guards the atomic-write path: a successful
// merge must not leave scratch files next to the settings file.
func TestApplySettingsLeavesNoTempFiles(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "settings")
	destDir := filepath.Join(tempDir, ".claude")
	for _, d := range []string{srcDir, destDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(srcDir, "claude.json"),
		[]byte(`{"enabledPlugins": {"a@m": true}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destDir, "settings.json"),
		[]byte(`{"theme": "light"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := ApplySettings(srcDir, destDir, config.PlatformClaude); err != nil {
		t.Fatalf("ApplySettings: %v", err)
	}

	entries, err := os.ReadDir(destDir)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"settings.json": true, "settings.json.bak": true}
	for _, e := range entries {
		if !want[e.Name()] {
			t.Errorf("unexpected leftover file %q in destination", e.Name())
		}
	}
}

// TestSettingsSourceFile pins the fragment naming so the status path and the
// apply path cannot drift apart — both must call this one function.
func TestSettingsSourceFile(t *testing.T) {
	if got := SettingsSourceFile(config.PlatformClaude); got != "claude.json" {
		t.Errorf("SettingsSourceFile(Claude) = %q; want claude.json", got)
	}
	if got := SettingsSourceFile(config.PlatformCodex); got != "" {
		t.Errorf("SettingsSourceFile(Codex) = %q; want \"\"", got)
	}
}

// TestApplySettings exercises the on-disk path: merge the fragment into the
// live file and report the outcome.
func TestApplySettings(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "settings")
	destDir := filepath.Join(tempDir, ".claude")
	for _, d := range []string{srcDir, destDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(srcDir, "claude.json"),
		[]byte(`{"enabledPlugins": {"a@m": true}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	destPath := filepath.Join(destDir, "settings.json")
	if err := os.WriteFile(destPath, []byte(`{"theme": "light"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := ApplySettings(srcDir, destDir, config.PlatformClaude)
	if err != nil {
		t.Fatalf("ApplySettings: %v", err)
	}
	if res.Merged != 1 {
		t.Errorf("Merged = %d; want 1", res.Merged)
	}

	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("invalid JSON written: %v", err)
	}
	if got["theme"] != "light" {
		t.Errorf("theme = %v; want light preserved", got["theme"])
	}
	if _, exists := got["enabledPlugins"]; !exists {
		t.Error("enabledPlugins not injected")
	}

	// Re-applying is a no-op.
	res2, err := ApplySettings(srcDir, destDir, config.PlatformClaude)
	if err != nil {
		t.Fatalf("ApplySettings (second run): %v", err)
	}
	if res2.Merged != 0 {
		t.Errorf("second run Merged = %d; want 0 (idempotent)", res2.Merged)
	}
}
