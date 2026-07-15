// ABOUTME: TOML variant of settings-fragment injection, for Codex's config.toml.
// ABOUTME: Owns only top-level bare keys; every [table] section is preserved byte-for-byte.

package sync

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

// A top-level key assignment: an optional-indent key (bare, dotted, or quoted),
// then '='. TOML top-level keys can only appear before the first [table] header,
// so the whole owned region is the file's preamble.
var tomlAssignRe = regexp.MustCompile(`^[ \t]*("(?:[^"\\]|\\.)*"|'[^']*'|[A-Za-z0-9_.-]+)[ \t]*=`)

// tomlTableRe matches a table or array-of-tables header line ([x] or [[x]]).
var tomlTableRe = regexp.MustCompile(`^[ \t]*\[`)

// stripTomlInlineComment returns s truncated at the first '#' that is not inside
// a quoted string, right-trimmed. TOML comments run to end-of-line unless the
// '#' sits inside a basic ("...") or literal ('...') string.
func stripTomlInlineComment(s string) string {
	inBasic, inLiteral, esc := false, false, false
	for i, r := range s {
		switch {
		case inBasic:
			if esc {
				esc = false
			} else if r == '\\' {
				esc = true
			} else if r == '"' {
				inBasic = false
			}
		case inLiteral:
			if r == '\'' {
				inLiteral = false
			}
		case r == '"':
			inBasic = true
		case r == '\'':
			inLiteral = true
		case r == '#':
			return strings.TrimSpace(s[:i])
		}
	}
	return strings.TrimSpace(s)
}

// tomlOwnedKeys extracts the ordered (key, valueText) pairs a fragment declares.
// The fragment must contain only single-line top-level keys — a table header
// means it is trying to own a machine-specific section, and a multi-line value
// (an unterminated `"""`/`”'`) cannot be edited by line surgery — so both are
// rejected. The value's inline comment is stripped; the remaining text passes
// through verbatim, so no TOML type parsing is needed.
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
		val := stripTomlInlineComment(line[strings.Index(line, "=")+1:])
		if strings.Contains(val, `"""`) || strings.Contains(val, "'''") {
			return nil, fmt.Errorf("settings fragment value for %q looks multi-line; only single-line values are supported", m[1])
		}
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
// Line-based surgery cannot see into a top-level multi-line string ("""..."""),
// whose continuation lines can start with '[' and masquerade as table headers,
// so if the preamble contains a triple-quote the merge REFUSES rather than risk
// corrupting the file. Codex's config.toml has no such strings; this is a safe
// backstop, not an expected path. Multi-line strings inside a [table] are fine —
// they live in the preserved-verbatim region and are never parsed.
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
//	err     (error):  A table/multi-line in the fragment, a malformed fragment
//	                  line, or a top-level multi-line string in the live file.
func MergeTomlSettingsContent(live, fragment []byte) ([]byte, bool, error) {
	owned, err := tomlOwnedKeys(fragment)
	if err != nil {
		return nil, false, err
	}

	// Split live into the preamble (top-level keys) and the tables region, which
	// starts at the first table header and is preserved verbatim.
	lines := strings.SplitAfter(string(live), "\n") // keep line endings
	var preamble []string
	tablesFrom := -1
	for i, l := range lines {
		if tomlTableRe.MatchString(l) {
			tablesFrom = i
			break
		}
		preamble = append(preamble, l)
	}

	// Refuse if the preamble holds a multi-line string: a '[' inside it may have
	// been misread as the table boundary above, so line surgery is unsafe.
	for _, l := range preamble {
		if strings.Contains(l, `"""`) || strings.Contains(l, "'''") {
			return nil, false, fmt.Errorf(
				"config.toml top-level region contains a multi-line string; refusing to edit it safely")
		}
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
