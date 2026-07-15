package sync

import (
	"bytes"
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
	if got := SettingsTargetName(config.PlatformCodex); got != "config.toml" {
		t.Errorf("SettingsTargetName(Codex) = %q; want config.toml", got)
	}
	// Platforms without a supported settings file yield "".
	for _, p := range []config.Platform{config.PlatformAntigravity, config.PlatformOpencode} {
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

// TestMergeSettingsContentPreservesKeyOrder verifies the live file's key order
// survives a merge: owned keys are replaced where they already sit and new ones
// are appended. Re-sorting the file would produce a huge spurious diff in a
// dotfiles repo on the very first run.
func TestMergeSettingsContentPreservesKeyOrder(t *testing.T) {
	live := []byte(`{
  "zebra": 1,
  "enabledPlugins": {"a@m": true},
  "alpha": 2
}`)
	fragment := []byte(`{"enabledPlugins": {"b@m": true}, "extraKnownMarketplaces": {"m": {}}}`)

	merged, _, err := MergeSettingsContent(live, fragment)
	if err != nil {
		t.Fatalf("MergeSettingsContent: %v", err)
	}

	got, err := decodeOrderedObject(merged)
	if err != nil {
		t.Fatalf("merged output is not a JSON object: %v", err)
	}
	var keys []string
	for _, f := range got {
		keys = append(keys, f.Key)
	}

	want := []string{"zebra", "enabledPlugins", "alpha", "extraKnownMarketplaces"}
	if len(keys) != len(want) {
		t.Fatalf("keys = %v; want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Errorf("key[%d] = %q; want %q (live order must be preserved, new keys appended)", i, keys[i], want[i])
		}
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

// TestMergeSettingsContentNullLive verifies that a settings file containing a
// bare `null` is rejected rather than panicking on a nil map assignment.
func TestMergeSettingsContentNullLive(t *testing.T) {
	if _, _, err := MergeSettingsContent([]byte(`null`), []byte(`{"a": 1}`)); err == nil {
		t.Error("expected an error for a null live settings file, got nil")
	}
	if _, _, err := MergeSettingsContent([]byte(`{}`), []byte(`null`)); err == nil {
		t.Error("expected an error for a null fragment, got nil")
	}
}

// TestApplySettingsWritesThroughSymlink verifies that a dotfiles-managed
// settings.json (a symlink) keeps its link: the merge must write through to the
// link target instead of replacing the link with a regular file.
func TestApplySettingsWritesThroughSymlink(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "settings")
	destDir := filepath.Join(tempDir, ".claude")
	store := filepath.Join(tempDir, "dotfiles")
	for _, d := range []string{srcDir, destDir, store} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(srcDir, "claude.json"),
		[]byte(`{"enabledPlugins": {"a@m": true}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// The real file lives in the dotfiles store; ~/.claude/settings.json links to it.
	real := filepath.Join(store, "claude-settings.json")
	if err := os.WriteFile(real, []byte(`{"theme": "light"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(destDir, "settings.json")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}

	if _, err := ApplySettings(srcDir, destDir, config.PlatformClaude); err != nil {
		t.Fatalf("ApplySettings: %v", err)
	}

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("settings.json is no longer a symlink; the link was replaced by a regular file")
	}

	data, err := os.ReadFile(real)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("link target is not valid JSON: %v", err)
	}
	if _, exists := got["enabledPlugins"]; !exists {
		t.Error("fragment was not written through to the link target")
	}
	if got["theme"] != "light" {
		t.Error("link target lost its unowned keys")
	}
}

// TestApplySettingsReplacesPermissiveBackup verifies that a stale world-readable
// .bak is not merely truncated (which would keep its mode) but replaced 0600.
func TestApplySettingsReplacesPermissiveBackup(t *testing.T) {
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
	// A stale, world-readable backup from some earlier manual copy.
	bak := filepath.Join(destDir, "settings.json.bak")
	if err := os.WriteFile(bak, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ApplySettings(srcDir, destDir, config.PlatformClaude); err != nil {
		t.Fatalf("ApplySettings: %v", err)
	}

	info, err := os.Stat(bak)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("backup mode = %#o; want 0600 (a stale permissive backup must be replaced, not truncated)", got)
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
	if got := SettingsSourceFile(config.PlatformCodex); got != "codex.toml" {
		t.Errorf("SettingsSourceFile(Codex) = %q; want codex.toml", got)
	}
	if got := SettingsSourceFile(config.PlatformOpencode); got != "" {
		t.Errorf("SettingsSourceFile(Opencode) = %q; want \"\"", got)
	}
}

// TestCaptureSettingsContent verifies the reverse direction: the fragment picks
// up the live value of every key it already owns, and nothing else. Machine-local
// keys must never leak into the version-controlled fragment.
func TestCaptureSettingsContent(t *testing.T) {
	fragment := []byte(`{"enabledPlugins": {"a@m": true}, "extraKnownMarketplaces": {"m": {"repo": "x/y"}}}`)
	live := []byte(`{
  "theme": "light",
  "hooks": {"PostToolUse": []},
  "enabledPlugins": {"a@m": true, "new@m": true},
  "extraKnownMarketplaces": {"m": {"repo": "x/y"}}
}`)

	captured, changed, err := CaptureSettingsContent(live, fragment)
	if err != nil {
		t.Fatalf("CaptureSettingsContent: %v", err)
	}
	if !changed {
		t.Error("changed = false; want true (a new plugin appeared live)")
	}

	var got map[string]any
	if err := json.Unmarshal(captured, &got); err != nil {
		t.Fatalf("captured fragment is not valid JSON: %v", err)
	}

	// Only the owned keys are present — no machine-local leakage.
	if _, leaked := got["theme"]; leaked {
		t.Error("theme leaked into the fragment; capture must only touch owned keys")
	}
	if _, leaked := got["hooks"]; leaked {
		t.Error("hooks leaked into the fragment")
	}
	if len(got) != 2 {
		t.Errorf("fragment has %d keys (%v); want exactly the 2 owned keys", len(got), got)
	}

	// The owned key picked up the live addition.
	plugins, ok := got["enabledPlugins"].(map[string]any)
	if !ok {
		t.Fatalf("enabledPlugins = %v; want object", got["enabledPlugins"])
	}
	if plugins["new@m"] != true {
		t.Errorf("enabledPlugins = %v; want the live-added new@m captured", plugins)
	}
}

// TestCaptureSettingsContentNoChange verifies capture is a no-op when the live
// file already agrees with the fragment.
func TestCaptureSettingsContentNoChange(t *testing.T) {
	fragment := []byte(`{"enabledPlugins": {"a@m": true}}`)
	live := []byte(`{"theme": "light", "enabledPlugins": {"a@m": true}}`)

	_, changed, err := CaptureSettingsContent(live, fragment)
	if err != nil {
		t.Fatalf("CaptureSettingsContent: %v", err)
	}
	if changed {
		t.Error("changed = true; want false when live already matches the fragment")
	}
}

// TestCaptureSettingsContentDroppedKey verifies that an owned key which no
// longer exists live is dropped from the fragment, so disabling a plugin
// through the agent's own UI is captured too.
func TestCaptureSettingsContentDroppedKey(t *testing.T) {
	fragment := []byte(`{"enabledPlugins": {"a@m": true}, "extraKnownMarketplaces": {"m": {}}}`)
	live := []byte(`{"enabledPlugins": {"a@m": true}}`)

	captured, changed, err := CaptureSettingsContent(live, fragment)
	if err != nil {
		t.Fatalf("CaptureSettingsContent: %v", err)
	}
	if !changed {
		t.Error("changed = false; want true (an owned key vanished live)")
	}
	var got map[string]any
	if err := json.Unmarshal(captured, &got); err != nil {
		t.Fatal(err)
	}
	if _, exists := got["extraKnownMarketplaces"]; exists {
		t.Error("extraKnownMarketplaces still in fragment; an owned key absent live must be dropped")
	}
}

// TestCaptureSettingsRefusesWhenLiveAbsent is the guard against the worst
// failure mode: on a machine where the agent has never written its settings
// file, treating "absent" as an empty object would make every owned key look
// removed and blank the version-controlled fragment.
func TestCaptureSettingsRefusesWhenLiveAbsent(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "settings")
	destDir := filepath.Join(tempDir, ".claude") // exists, but has no settings.json
	for _, d := range []string{srcDir, destDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	fragPath := filepath.Join(srcDir, "claude.json")
	original := []byte(`{"enabledPlugins": {"a@m": true}}`)
	if err := os.WriteFile(fragPath, original, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := CaptureSettings(srcDir, destDir, config.PlatformClaude); err == nil {
		t.Error("expected an error when the live settings file is absent, got nil")
	}

	after, err := os.ReadFile(fragPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, original) {
		t.Errorf("fragment was modified (%s); it must be left untouched", after)
	}
}

// TestApplySettingsSymlinkWithMissingTarget covers the dotfiles setup where the
// symlink is created before its target file exists: the link must be preserved
// and the target created, not the link replaced by a regular file.
func TestApplySettingsSymlinkWithMissingTarget(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "settings")
	destDir := filepath.Join(tempDir, ".claude")
	store := filepath.Join(tempDir, "dotfiles")
	for _, d := range []string{srcDir, destDir, store} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(srcDir, "claude.json"),
		[]byte(`{"enabledPlugins": {"a@m": true}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	real := filepath.Join(store, "claude-settings.json") // deliberately not created
	link := filepath.Join(destDir, "settings.json")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}

	if _, err := ApplySettings(srcDir, destDir, config.PlatformClaude); err != nil {
		t.Fatalf("ApplySettings: %v", err)
	}

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the symlink was replaced by a regular file")
	}
	data, err := os.ReadFile(real)
	if err != nil {
		t.Fatalf("link target was not created: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("target is not valid JSON: %v", err)
	}
	if _, exists := got["enabledPlugins"]; !exists {
		t.Error("fragment was not written to the link target")
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
