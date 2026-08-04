// ABOUTME: Reads and atomically writes the small TOML manifest that persists policy-module selections.
// ABOUTME: The supported schema is intentionally limited to version and a flat ordered module ID array.

package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var manifestAssignmentPattern = regexp.MustCompile(`^([A-Za-z0-9_-]+)\s*=\s*(.*)$`)

// PolicyManifest is the persisted module selection for one sync scope.
type PolicyManifest struct {
	Version int
	Modules []string
}

func parseManifestModuleList(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if len(value) < 2 || value[0] != '[' || value[len(value)-1] != ']' {
		return nil, fmt.Errorf("modules must be a TOML array")
	}
	inner := strings.TrimSpace(value[1 : len(value)-1])
	if inner == "" {
		return []string{}, nil
	}
	var modules []string
	for len(inner) > 0 {
		if inner[0] != '"' {
			return nil, fmt.Errorf("module ids must be quoted strings")
		}
		end := 1
		for end < len(inner) {
			if inner[end] == '"' && inner[end-1] != '\\' {
				break
			}
			end++
		}
		if end >= len(inner) {
			return nil, fmt.Errorf("unterminated module id")
		}
		decoded, err := strconv.Unquote(inner[:end+1])
		if err != nil || !policyModuleIDPattern.MatchString(decoded) {
			return nil, fmt.Errorf("invalid module id %q", decoded)
		}
		modules = append(modules, decoded)
		inner = strings.TrimSpace(inner[end+1:])
		if inner == "" {
			break
		}
		if inner[0] != ',' {
			return nil, fmt.Errorf("expected comma between module ids")
		}
		inner = strings.TrimSpace(inner[1:])
		if inner == "" {
			return nil, fmt.Errorf("trailing comma is not supported")
		}
	}
	return modules, nil
}

// ReadPolicyManifest returns exists=false for a missing manifest and validates
// the complete version-1 schema when the file is present.
func ReadPolicyManifest(path string) (PolicyManifest, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return PolicyManifest{}, false, nil
	}
	if err != nil {
		return PolicyManifest{}, false, err
	}
	manifest := PolicyManifest{}
	seenKeys := make(map[string]bool)
	for lineNumber, rawLine := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		match := manifestAssignmentPattern.FindStringSubmatch(line)
		if match == nil {
			return PolicyManifest{}, true, fmt.Errorf("%s:%d: invalid manifest line", path, lineNumber+1)
		}
		key, value := match[1], match[2]
		if seenKeys[key] {
			return PolicyManifest{}, true, fmt.Errorf("%s: duplicate key %q", path, key)
		}
		seenKeys[key] = true
		switch key {
		case "version":
			manifest.Version, err = strconv.Atoi(strings.TrimSpace(value))
		case "modules":
			manifest.Modules, err = parseManifestModuleList(value)
		default:
			return PolicyManifest{}, true, fmt.Errorf("%s: unsupported key %q", path, key)
		}
		if err != nil {
			return PolicyManifest{}, true, fmt.Errorf("%s: invalid %s: %w", path, key, err)
		}
	}
	if manifest.Version != 1 {
		return PolicyManifest{}, true, fmt.Errorf("%s: unsupported manifest version %d", path, manifest.Version)
	}
	if !seenKeys["modules"] {
		return PolicyManifest{}, true, fmt.Errorf("%s: modules is required", path)
	}
	seenModules := make(map[string]bool)
	for _, id := range manifest.Modules {
		if seenModules[id] {
			return PolicyManifest{}, true, fmt.Errorf("%s: duplicate module %q", path, id)
		}
		seenModules[id] = true
	}
	return manifest, true, nil
}

// WritePolicyManifest validates and atomically writes a version-1 manifest.
func WritePolicyManifest(path string, manifest PolicyManifest) error {
	if manifest.Version != 1 {
		return fmt.Errorf("unsupported manifest version %d", manifest.Version)
	}
	seen := make(map[string]bool)
	quoted := make([]string, 0, len(manifest.Modules))
	for _, id := range manifest.Modules {
		if !policyModuleIDPattern.MatchString(id) {
			return fmt.Errorf("invalid module id %q", id)
		}
		if seen[id] {
			return fmt.Errorf("duplicate module %q", id)
		}
		seen[id] = true
		quoted = append(quoted, strconv.Quote(id))
	}
	content := fmt.Sprintf("version = 1\nmodules = [%s]\n", strings.Join(quoted, ", "))
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".policy.toml-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
