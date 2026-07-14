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

// SettingsSourceFile returns the fragment file name under the settings source
// directory for a platform, or "" if unsupported. Both the apply path and the
// status path must use this single definition so they cannot drift apart.
func SettingsSourceFile(platform config.Platform) string {
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
		// A bare `null` unmarshals without error but leaves the map nil, which
		// would panic on assignment below. It is not a settings object.
		if liveMap == nil {
			return nil, false, fmt.Errorf("parse live settings: expected a JSON object, got null")
		}
	}

	fragMap := map[string]json.RawMessage{}
	if err := json.Unmarshal(fragment, &fragMap); err != nil {
		return nil, false, fmt.Errorf("parse settings fragment: %w", err)
	}
	if fragMap == nil {
		return nil, false, fmt.Errorf("parse settings fragment: expected a JSON object, got null")
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

// readSettings reads the live settings file, treating "not found" as empty.
// It also reports the file's permission bits so a rewrite can preserve them.
func readSettings(path string) (content []byte, mode os.FileMode, err error) {
	// Settings can hold tokens and machine config, so a file we create is
	// owner-only. An existing file keeps whatever mode it already has — the
	// tool owns certain keys, not the file's permissions.
	mode = 0o600
	info, err := os.Stat(path)
	if err == nil {
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return nil, mode, err
	}

	content, err = os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, mode, nil
		}
		return nil, mode, err
	}
	return content, mode, nil
}

// settingsWriteAttempts bounds the compare-and-swap retry loop in ApplySettings.
const settingsWriteAttempts = 3

// ApplySettings merges the platform's fragment from srcDir into its live
// settings file under destDir.
//
// The live file is shared with the agent itself — Claude Code rewrites
// settings.json when the user changes model, effort, theme, and so on — so the
// write is defensive on three counts:
//
//   - Atomic: the merged content is written to a temp file in the same
//     directory and renamed over the target, so a crash or full disk can never
//     leave a truncated settings file behind.
//   - Compare-and-swap: the file is re-read immediately before the rename and
//     the merge is redone if it changed underneath us, so a concurrent write is
//     folded in rather than clobbered. This narrows but cannot fully close the
//     race, since the agent does not take a lock we could share.
//   - Backed up: the exact content being replaced is copied to <file>.bak first,
//     and the write is abandoned if that backup cannot be made.
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
	srcName := SettingsSourceFile(platform)
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

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return res, fmt.Errorf("mkdir -p %s: %w", destDir, err)
	}
	destPath := filepath.Join(destDir, target)

	// If the settings file is a symlink (a dotfiles-managed setup), write
	// through to its target. Renaming onto the link path would replace the link
	// with a regular file and silently detach the user's tracked settings.
	if resolved, err := filepath.EvalSymlinks(destPath); err == nil {
		destPath = resolved
	} else if !os.IsNotExist(err) {
		return res, fmt.Errorf("resolve %s: %w", destPath, err)
	}
	writeDir := filepath.Dir(destPath)

	for attempt := 1; ; attempt++ {
		live, mode, err := readSettings(destPath)
		if err != nil {
			return res, fmt.Errorf("read %s: %w", destPath, err)
		}

		merged, changed, err := MergeSettingsContent(live, fragment)
		if err != nil {
			return res, fmt.Errorf("%s: %w", destPath, err)
		}
		if !changed {
			return res, nil // already applied: write nothing at all
		}

		// Stage the new content beside the target so the rename is atomic (and
		// lands on the same filesystem).
		tmp, err := os.CreateTemp(writeDir, target+".tmp*")
		if err != nil {
			return res, fmt.Errorf("stage %s: %w", destPath, err)
		}
		tmpPath := tmp.Name()
		// Close explicitly rather than deferring: filesystems such as NFS and
		// quota-backed mounts surface write failures only on Close, and renaming
		// a partially written file would defeat the point of staging it.
		writeErr := func() error {
			if _, err := tmp.Write(merged); err != nil {
				tmp.Close()
				return err
			}
			if err := tmp.Chmod(mode); err != nil {
				tmp.Close()
				return err
			}
			return tmp.Close()
		}()
		if writeErr != nil {
			os.Remove(tmpPath)
			return res, fmt.Errorf("stage %s: %w", destPath, writeErr)
		}

		// Re-read just before committing: if the agent rewrote the file while we
		// were merging, redo the merge on top of its version instead of
		// discarding it.
		current, _, err := readSettings(destPath)
		if err != nil {
			os.Remove(tmpPath)
			return res, fmt.Errorf("re-read %s: %w", destPath, err)
		}
		if !bytes.Equal(current, live) {
			os.Remove(tmpPath)
			if attempt < settingsWriteAttempts {
				continue
			}
			return res, fmt.Errorf("%s kept changing underneath us after %d attempts; not writing",
				destPath, settingsWriteAttempts)
		}

		if len(live) > 0 {
			// Remove any stale backup first: os.WriteFile does not apply its mode
			// to an existing file, so a world-readable leftover .bak would keep
			// its permissions while receiving the settings content.
			bakPath := destPath + ".bak"
			if err := os.Remove(bakPath); err != nil && !os.IsNotExist(err) {
				os.Remove(tmpPath)
				return res, fmt.Errorf("replace stale backup %s: %w", bakPath, err)
			}
			if err := os.WriteFile(bakPath, live, 0o600); err != nil {
				os.Remove(tmpPath)
				return res, fmt.Errorf("back up %s: %w", destPath, err)
			}
			fmt.Printf("  backed up: %s\n", bakPath)
		}

		if err := os.Rename(tmpPath, destPath); err != nil {
			os.Remove(tmpPath)
			return res, fmt.Errorf("write %s: %w", destPath, err)
		}

		fmt.Printf("  merged: %s\n", destPath)
		res.Merged++
		return res, nil
	}
}
