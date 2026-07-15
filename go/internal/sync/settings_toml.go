// ABOUTME: TOML variant of settings-fragment injection, for Codex's config.toml.
// ABOUTME: Owns only top-level bare keys; every [table] section is preserved byte-for-byte.

package sync

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

// A top-level key assignment: an optional-indent key, then '='. TOML top-level
// keys can only appear before the first [table] header, so the whole owned
// region is the file's preamble.
var tomlAssignRe = regexp.MustCompile(`^[ \t]*([A-Za-z0-9_.-]+)[ \t]*=`)

// tomlTableRe matches a table or array-of-tables header line ([x] or [[x]]).
var tomlTableRe = regexp.MustCompile(`^[ \t]*\[`)

// tomlOwnedKeys extracts the ordered (key, valueText) pairs a fragment declares.
// The fragment must contain only top-level keys — a table header means it is
// trying to own a machine-specific section, which is rejected. valueText is the
// verbatim right-hand side (quotes, arrays, and all), so no TOML type parsing is
// needed and unusual values pass through unchanged.
func tomlOwnedKeys(fragment []byte) ([]struct{ key, val string }, error) {
	var owned []struct{ key, val string }
	for _, raw := range strings.Split(string(fragment), "\n") {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if tomlTableRe.MatchString(line) {
			return nil, fmt.Errorf("settings fragment must contain only top-level keys, found a table: %q", trimmed)
		}
		m := tomlAssignRe.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("settings fragment has a line that is not a top-level key assignment: %q", trimmed)
		}
		val := strings.TrimSpace(line[strings.Index(line, "=")+1:])
		owned = append(owned, struct{ key, val string }{m[1], val})
	}
	return owned, nil
}

// MergeTomlSettingsContent injects a TOML fragment into a live config.toml with
// the same ownership semantics as the JSON path: a top-level key present in the
// fragment replaces the live value, keys absent are preserved, and — crucially —
// every [table] section (Codex's per-project trust, hook trust hashes, TUI
// counters) is copied through byte-for-byte. Only the file's preamble (the
// region before the first table header, where top-level keys legally live) is
// rewritten; new owned keys are appended to the end of that preamble.
//
// Args:
//
//	live     ([]byte): Current config.toml content; empty/nil if absent.
//	fragment ([]byte): The version-controlled fragment (top-level keys only).
//
// Returns:
//
//	merged  ([]byte): The content to write.
//	changed (bool):   False if live already equals merged.
//	err     (error):  A table in the fragment, or a malformed fragment line.
func MergeTomlSettingsContent(live, fragment []byte) ([]byte, bool, error) {
	owned, err := tomlOwnedKeys(fragment)
	if err != nil {
		return nil, false, err
	}

	// Split live into the preamble (top-level keys) and the tables region, which
	// starts at the first table header and is preserved verbatim.
	liveStr := string(live)
	lines := strings.SplitAfter(liveStr, "\n") // keep line endings
	var preamble []string
	tablesFrom := -1
	for i, l := range lines {
		if tomlTableRe.MatchString(l) {
			tablesFrom = i
			break
		}
		preamble = append(preamble, l)
	}
	tables := ""
	if tablesFrom >= 0 {
		tables = strings.Join(lines[tablesFrom:], "")
	}

	// Rewrite owned keys in place; remember which were seen so the rest can be
	// appended.
	seen := make(map[string]bool, len(owned))
	valOf := make(map[string]string, len(owned))
	for _, o := range owned {
		valOf[o.key] = o.val // last-wins on a duplicate fragment key
	}
	for i, l := range preamble {
		m := tomlAssignRe.FindStringSubmatch(l)
		if m == nil {
			continue // comment, blank, or continuation line
		}
		if v, ok := valOf[m[1]]; ok {
			eol := ""
			if strings.HasSuffix(l, "\n") {
				eol = "\n"
			}
			preamble[i] = fmt.Sprintf("%s = %s%s", m[1], v, eol)
			seen[m[1]] = true
		}
	}

	// Append owned keys that were not already present, keeping fragment order.
	var appended strings.Builder
	for _, o := range owned {
		if !seen[o.key] {
			appended.WriteString(fmt.Sprintf("%s = %s\n", o.key, o.val))
			seen[o.key] = true
		}
	}

	var out bytes.Buffer
	pre := strings.Join(preamble, "")
	out.WriteString(pre)
	if appended.Len() > 0 {
		// Ensure the appended block starts on its own line even if the preamble
		// did not end with a newline.
		if pre != "" && !strings.HasSuffix(pre, "\n") {
			out.WriteString("\n")
		}
		out.WriteString(appended.String())
	}
	out.WriteString(tables)

	merged := out.Bytes()
	return merged, !bytes.Equal(merged, live), nil
}
