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

// jsonField is one top-level key/value pair of a settings document, kept in
// file order.
type jsonField struct {
	Key string
	Val json.RawMessage
}

// decodeOrderedObject decodes a JSON object preserving its key order. Go's maps
// are unordered and MarshalIndent sorts keys, which would rewrite the user's
// whole settings file on first run; keeping the original order means only the
// keys we actually own show up in a diff. A duplicate key keeps its first
// position and its last value, matching encoding/json's last-wins rule.
func decodeOrderedObject(data []byte) ([]jsonField, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if tok != json.Delim('{') {
		return nil, fmt.Errorf("expected a JSON object")
	}

	var fields []jsonField
	index := map[string]int{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("expected an object key, got %v", keyTok)
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		if i, seen := index[key]; seen {
			fields[i].Val = raw
			continue
		}
		index[key] = len(fields)
		fields = append(fields, jsonField{Key: key, Val: raw})
	}
	if _, err := dec.Token(); err != nil { // closing brace
		return nil, err
	}
	return fields, nil
}

// encodeOrderedObject renders fields as a 2-space-indented JSON object with a
// trailing newline, preserving the given key order.
func encodeOrderedObject(fields []jsonField) ([]byte, error) {
	if len(fields) == 0 {
		return []byte("{}\n"), nil
	}
	var buf bytes.Buffer
	buf.WriteString("{\n")
	for i, f := range fields {
		key, err := json.Marshal(f.Key)
		if err != nil {
			return nil, err
		}
		var compact bytes.Buffer
		if err := json.Compact(&compact, f.Val); err != nil {
			return nil, err
		}
		var value bytes.Buffer
		if err := json.Indent(&value, compact.Bytes(), "  ", "  "); err != nil {
			return nil, err
		}
		buf.WriteString("  ")
		buf.Write(key)
		buf.WriteString(": ")
		buf.Write(value.Bytes())
		if i < len(fields)-1 {
			buf.WriteString(",")
		}
		buf.WriteString("\n")
	}
	buf.WriteString("}\n")
	return buf.Bytes(), nil
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
	var liveFields []jsonField
	if len(bytes.TrimSpace(live)) > 0 {
		var err error
		if liveFields, err = decodeOrderedObject(live); err != nil {
			return nil, false, fmt.Errorf("parse live settings: %w", err)
		}
	}

	fragFields, err := decodeOrderedObject(fragment)
	if err != nil {
		return nil, false, fmt.Errorf("parse settings fragment: %w", err)
	}
	owned := make(map[string]json.RawMessage, len(fragFields))
	for _, f := range fragFields {
		owned[f.Key] = f.Val
	}

	// Owned keys are replaced where they already sit; new ones are appended, so
	// the live file's existing key order survives.
	merged := make([]jsonField, 0, len(liveFields)+len(fragFields))
	present := make(map[string]bool, len(liveFields))
	for _, f := range liveFields {
		present[f.Key] = true
		if v, isOwned := owned[f.Key]; isOwned {
			f.Val = v
		}
		merged = append(merged, f)
	}
	for _, f := range fragFields {
		if !present[f.Key] {
			merged = append(merged, f)
		}
	}

	out, err := encodeOrderedObject(merged)
	if err != nil {
		return nil, false, fmt.Errorf("encode merged settings: %w", err)
	}

	return out, !equalJSON(live, out), nil
}

// CaptureSettingsContent is the reverse of MergeSettingsContent: it rewrites the
// fragment so each key it already owns takes the live file's current value.
//
// This closes the loop for changes made through the agent's own UI — installing
// a plugin with Claude Code's `/plugin` command writes `enabledPlugins` in the
// live file, and without capturing it back the next `--refresh` would remove the
// new plugin again, since the fragment owns that key wholesale.
//
// Only keys already present in the fragment are touched: the fragment declares
// what it owns, and capture never widens that set, so machine-local settings
// (theme, hooks, permissions, …) can never leak into version control. An owned
// key that no longer exists in the live file is dropped from the fragment.
//
// Args:
//
//	live     ([]byte): Current settings file content.
//	fragment ([]byte): The version-controlled fragment, defining the owned keys.
//
// Returns:
//
//	captured ([]byte): The fragment content to write.
//	changed  (bool):   False if the fragment already matches the live values.
//	err      (error):  Malformed JSON in either input, or nil.
func CaptureSettingsContent(live, fragment []byte) ([]byte, bool, error) {
	var liveFields []jsonField
	if len(bytes.TrimSpace(live)) > 0 {
		var err error
		if liveFields, err = decodeOrderedObject(live); err != nil {
			return nil, false, fmt.Errorf("parse live settings: %w", err)
		}
	}
	liveByKey := make(map[string]json.RawMessage, len(liveFields))
	for _, f := range liveFields {
		liveByKey[f.Key] = f.Val
	}

	fragFields, err := decodeOrderedObject(fragment)
	if err != nil {
		return nil, false, fmt.Errorf("parse settings fragment: %w", err)
	}

	// Walk the fragment's own key order: it defines what is owned, and capture
	// never widens that set. An owned key absent from the live file is dropped.
	captured := make([]jsonField, 0, len(fragFields))
	for _, f := range fragFields {
		if v, exists := liveByKey[f.Key]; exists {
			captured = append(captured, jsonField{Key: f.Key, Val: v})
		}
	}

	out, err := encodeOrderedObject(captured)
	if err != nil {
		return nil, false, fmt.Errorf("encode captured fragment: %w", err)
	}

	return out, !equalJSON(fragment, out), nil
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

// CaptureSettings rewrites the platform's fragment in srcDir so its owned keys
// take the values from the live settings file under destDir. Use it after
// changing settings through the agent's own UI (e.g. Claude Code's `/plugin`),
// so the version-controlled fragment — which `--refresh` treats as the source of
// truth — does not undo the change on its next run.
//
// Returns:
//
//	result (Result): Merged is 1 when the fragment was updated, 0 when already current.
//	err    (error):  Read/parse/write error, or nil.
func CaptureSettings(srcDir, destDir string, platform config.Platform) (Result, error) {
	var res Result

	target := SettingsTargetName(platform)
	srcName := SettingsSourceFile(platform)
	if target == "" || srcName == "" {
		return res, nil
	}

	fragPath := filepath.Join(srcDir, srcName)
	fragment, err := os.ReadFile(fragPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Nothing declared: capture has no key set to work from, and widening
			// it would dump the whole settings file into version control.
			return res, nil
		}
		return res, fmt.Errorf("read settings fragment: %w", err)
	}

	livePath, err := resolveSettingsPath(filepath.Join(destDir, target))
	if err != nil {
		return res, fmt.Errorf("resolve live settings: %w", err)
	}
	live, _, exists, err := readSettings(livePath)
	if err != nil {
		return res, fmt.Errorf("read live settings: %w", err)
	}
	// Refuse on an absent or empty live file rather than treating it as an empty
	// settings object: every owned key would look removed and the fragment would
	// be blanked, destroying the version-controlled plugin declarations.
	if !exists {
		return res, fmt.Errorf("live settings %s does not exist; nothing to capture", livePath)
	}
	if len(bytes.TrimSpace(live)) == 0 {
		return res, fmt.Errorf("live settings %s is empty; refusing to blank the fragment", livePath)
	}

	captured, changed, err := CaptureSettingsContent(live, fragment)
	if err != nil {
		return res, fmt.Errorf("%s: %w", fragPath, err)
	}
	if !changed {
		return res, nil
	}

	// Write through a symlinked fragment to its target: the source root is a
	// curation layer, so settings/claude.json is typically a link into the repo
	// that must actually receive the captured content and be committed.
	writePath, err := resolveSettingsPath(fragPath)
	if err != nil {
		return res, fmt.Errorf("resolve %s: %w", fragPath, err)
	}
	// Atomic: a truncated fragment would lose the very key set that tells a
	// later capture what it owns, and could not be reconstructed from the live file.
	if err := writeFileAtomic(writePath, captured, 0o644); err != nil {
		return res, fmt.Errorf("write %s: %w", writePath, err)
	}
	fragPath = writePath

	fmt.Printf("  captured: %s\n", fragPath)
	res.Merged++
	return res, nil
}

// SettingsFragmentExists reports whether a fragment is declared for the platform
// in srcDir, so callers can distinguish "nothing to apply" from "already applied".
func SettingsFragmentExists(srcDir string, platform config.Platform) bool {
	name := SettingsSourceFile(platform)
	if name == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(srcDir, name))
	return err == nil
}

// readSettings reads the live settings file. A missing file is not an error —
// it is reported via exists, because the two callers must treat it differently:
// injection creates the file, while capture must refuse to run (an absent live
// file would look like "every owned key was removed" and blank the fragment).
//
// Returns:
//
//	content ([]byte):      File content, nil when absent.
//	mode    (os.FileMode): Existing permission bits, or 0600 for a file we would create.
//	exists  (bool):        Whether the file is present.
//	err     (error):       Read error other than "not found", or nil.
func readSettings(path string) (content []byte, mode os.FileMode, exists bool, err error) {
	// Settings can hold tokens and machine config, so a file we create is
	// owner-only. An existing file keeps whatever mode it already has — the
	// tool owns certain keys, not the file's permissions.
	mode = 0o600

	content, err = os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, mode, false, nil
		}
		return nil, mode, false, err
	}

	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	}
	return content, mode, true, nil
}

// resolveSettingsPath follows a symlinked settings file to the path that should
// actually be written. Unlike filepath.EvalSymlinks it does not require the
// target to exist: a dotfiles setup may create the link before the file, and
// renaming onto the link would replace it with a regular file.
func resolveSettingsPath(path string) (string, error) {
	for hop := 0; hop < 10; hop++ {
		info, err := os.Lstat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return path, nil // absent (or a link to an absent target): write here
			}
			return "", err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return path, nil
		}
		target, err := os.Readlink(path)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		path = filepath.Clean(target)
	}
	return "", fmt.Errorf("too many symlink hops resolving settings path")
}

// writeFileAtomic stages content in the target's directory and renames it into
// place, so an interrupted or failing write can never leave a truncated file.
func writeFileAtomic(path string, content []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	// Close explicitly rather than deferring: filesystems such as NFS and
	// quota-backed mounts surface write failures only on Close, and renaming a
	// partially written file would defeat the point of staging it.
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
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

	// If the settings file is a symlink (a dotfiles-managed setup), write through
	// to its target — including when that target does not exist yet, since the
	// link is often created first. Renaming onto the link path would replace the
	// link with a regular file and silently detach the user's tracked settings.
	destPath, err := resolveSettingsPath(filepath.Join(destDir, target))
	if err != nil {
		return res, fmt.Errorf("resolve %s: %w", filepath.Join(destDir, target), err)
	}

	for attempt := 1; ; attempt++ {
		live, mode, _, err := readSettings(destPath)
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

		// Re-read just before committing: if the agent rewrote the file while we
		// were merging, redo the merge on top of its version instead of
		// discarding it.
		current, _, _, err := readSettings(destPath)
		if err != nil {
			return res, fmt.Errorf("re-read %s: %w", destPath, err)
		}
		if !bytes.Equal(current, live) {
			if attempt < settingsWriteAttempts {
				continue
			}
			return res, fmt.Errorf("%s kept changing underneath us after %d attempts; not writing",
				destPath, settingsWriteAttempts)
		}

		if len(live) > 0 {
			// Replace any stale backup rather than truncating it: os.WriteFile does
			// not apply its mode to an existing file, so a world-readable leftover
			// .bak would keep its permissions while receiving the settings content.
			bakPath := destPath + ".bak"
			if err := os.Remove(bakPath); err != nil && !os.IsNotExist(err) {
				return res, fmt.Errorf("replace stale backup %s: %w", bakPath, err)
			}
			if err := writeFileAtomic(bakPath, live, 0o600); err != nil {
				return res, fmt.Errorf("back up %s: %w", destPath, err)
			}
			fmt.Printf("  backed up: %s\n", bakPath)
		}

		if err := writeFileAtomic(destPath, merged, mode); err != nil {
			return res, fmt.Errorf("write %s: %w", destPath, err)
		}

		fmt.Printf("  merged: %s\n", destPath)
		res.Merged++
		return res, nil
	}
}
