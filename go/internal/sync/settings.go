// ABOUTME: Injects version-controlled settings fragments into a platform's live settings file.
// ABOUTME: The fragment owns whole top-level keys; every other key in the live file is preserved.

package sync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/k1064190/claude-agent-skill-sync-tool/go/internal/config"
)

// SettingsTargetName returns the settings file name for a platform, or "" if
// the platform has no settings file this tool manages.
//
// Only Claude is supported today. The other agents' settings are not
// interchangeable with it — Codex uses TOML (`config.toml`), Opencode declares
// plugins as an array in `opencode.json`, and Antigravity splits config across
// several files — so there is nothing shared to sync and each would need its
// own format-aware writer.
func SettingsTargetName(platform config.Platform) string {
	switch platform {
	case config.PlatformClaude:
		return "settings.json"
	default:
		return ""
	}
}

// settingsSourceFile returns the fragment file name under the settings source
// directory for a platform, or "" if unsupported.
func settingsSourceFile(platform config.Platform) string {
	switch platform {
	case config.PlatformClaude:
		return "claude.json"
	default:
		return ""
	}
}

// MergeSettingsContent injects a settings fragment into live settings content.
//
// Ownership semantics: every top-level key present in the fragment replaces the
// live value **wholesale**, and every key absent from the fragment is preserved
// untouched. Wholesale replacement (rather than a deep merge) is what lets a
// fragment express removal — dropping a plugin from the fragment's
// `enabledPlugins` object actually removes it — while leaving machine-local
// keys such as `theme`, `hooks`, and `permissions` alone so they never churn.
//
// Args:
//
//	live     ([]byte): Current settings file content; empty/nil if it does not exist.
//	fragment ([]byte): The version-controlled fragment to inject.
//
// Returns:
//
//	merged  ([]byte): The settings content to write (2-space indented, trailing newline).
//	changed (bool):   False if live already equals the merged result.
//	err     (error):  Malformed JSON in either input, or nil.
func MergeSettingsContent(live, fragment []byte) ([]byte, bool, error) {
	// Decode into ordered-agnostic maps of raw values: nested content is copied
	// through verbatim rather than re-shaped.
	liveMap := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(live)) > 0 {
		if err := json.Unmarshal(live, &liveMap); err != nil {
			return nil, false, fmt.Errorf("parse live settings: %w", err)
		}
	}

	fragMap := map[string]json.RawMessage{}
	if err := json.Unmarshal(fragment, &fragMap); err != nil {
		return nil, false, fmt.Errorf("parse settings fragment: %w", err)
	}

	for k, v := range fragMap {
		liveMap[k] = v
	}

	merged, err := json.MarshalIndent(liveMap, "", "  ")
	if err != nil {
		return nil, false, fmt.Errorf("encode merged settings: %w", err)
	}
	merged = append(merged, '\n')

	return merged, !equalJSON(live, merged), nil
}

// equalJSON reports whether two JSON documents are semantically identical,
// ignoring key order and formatting. Used so an already-applied fragment
// reports no change even when the live file's formatting differs.
func equalJSON(a, b []byte) bool {
	var av, bv any
	if err := json.Unmarshal(a, &av); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		return false
	}
	ab, err := json.Marshal(av)
	if err != nil {
		return false
	}
	bb, err := json.Marshal(bv)
	if err != nil {
		return false
	}
	return bytes.Equal(ab, bb)
}

// ApplySettings merges the platform's fragment from srcDir into its live
// settings file under destDir. It backs the live file up to <file>.bak before
// overwriting and aborts that write if the backup cannot be created, so a
// hand-maintained settings file is never destroyed without a copy.
//
// Args:
//
//	srcDir   (string):          Absolute path to the settings source directory.
//	destDir  (string):          Absolute path to the directory holding the live settings file.
//	platform (config.Platform): Target platform.
//
// Returns:
//
//	result (Result): Merged is 1 when the file was updated, 0 when already in sync or skipped.
//	err    (error):  Read/parse/write error, or nil.
func ApplySettings(srcDir, destDir string, platform config.Platform) (Result, error) {
	var res Result

	target := SettingsTargetName(platform)
	srcName := settingsSourceFile(platform)
	if target == "" || srcName == "" {
		return res, nil // platform has no managed settings file
	}

	fragment, err := os.ReadFile(filepath.Join(srcDir, srcName))
	if err != nil {
		if os.IsNotExist(err) {
			return res, nil // nothing declared for this platform
		}
		return res, fmt.Errorf("read settings fragment: %w", err)
	}

	destPath := filepath.Join(destDir, target)
	live, err := os.ReadFile(destPath)
	if err != nil && !os.IsNotExist(err) {
		return res, fmt.Errorf("read %s: %w", destPath, err)
	}

	merged, changed, err := MergeSettingsContent(live, fragment)
	if err != nil {
		return res, fmt.Errorf("%s: %w", destPath, err)
	}
	if !changed {
		return res, nil
	}

	if len(live) > 0 {
		if err := os.WriteFile(destPath+".bak", live, 0o600); err != nil {
			return res, fmt.Errorf("back up %s: %w", destPath, err)
		}
		fmt.Printf("  backed up: %s.bak\n", destPath)
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return res, fmt.Errorf("mkdir -p %s: %w", destDir, err)
	}
	// Settings can hold tokens and machine config: keep it owner-only.
	if err := os.WriteFile(destPath, merged, 0o600); err != nil {
		return res, fmt.Errorf("write %s: %w", destPath, err)
	}

	fmt.Printf("  merged: %s\n", destPath)
	res.Merged++
	return res, nil
}
