// ABOUTME: Loads selectable policy modules and renders their shared and platform-specific prompts.
// ABOUTME: Modules are deterministic, validated directories under templates/modules/<id>/.

package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/k1064190/claude-agent-skill-sync-tool/go/internal/config"
)

var policyModuleIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// PolicyModule is one independently selectable instruction feature.
type PolicyModule struct {
	ID          string
	Description string
	Default     bool
	Order       int
	Common      string
	Overlays    map[config.Platform]string
}

// PolicyModulesExist reports whether srcDir declares the modular template
// layout. Callers use it to preserve the legacy common+platform builder.
func PolicyModulesExist(srcDir string) bool {
	info, err := os.Stat(filepath.Join(srcDir, "modules"))
	return err == nil && info.IsDir()
}

// parsePolicyModule splits the required frontmatter metadata from the common
// prompt body in one module.md file.
func parsePolicyModule(path, directoryID string) (PolicyModule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PolicyModule{}, err
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) < 3 || lines[0] != "---" {
		return PolicyModule{}, fmt.Errorf("%s: missing YAML frontmatter", path)
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return PolicyModule{}, fmt.Errorf("%s: unterminated YAML frontmatter", path)
	}

	fields := make(map[string]string)
	for _, line := range lines[1:end] {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return PolicyModule{}, fmt.Errorf("%s: invalid frontmatter line %q", path, line)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if _, exists := fields[key]; exists {
			return PolicyModule{}, fmt.Errorf("%s: duplicate frontmatter key %q", path, key)
		}
		fields[key] = value
	}
	for key := range fields {
		switch key {
		case "id", "description", "default", "order":
		default:
			return PolicyModule{}, fmt.Errorf("%s: unsupported frontmatter key %q", path, key)
		}
	}

	id := fields["id"]
	if !policyModuleIDPattern.MatchString(id) {
		return PolicyModule{}, fmt.Errorf("%s: invalid module id %q", path, id)
	}
	if id != directoryID {
		return PolicyModule{}, fmt.Errorf("%s: module id %q does not match directory %q", path, id, directoryID)
	}
	description := fields["description"]
	if description == "" {
		return PolicyModule{}, fmt.Errorf("%s: description is required", path)
	}
	defaultOn, err := strconv.ParseBool(fields["default"])
	if err != nil {
		return PolicyModule{}, fmt.Errorf("%s: invalid default value: %w", path, err)
	}
	order, err := strconv.Atoi(fields["order"])
	if err != nil {
		return PolicyModule{}, fmt.Errorf("%s: invalid order value: %w", path, err)
	}
	body := strings.TrimSpace(strings.Join(lines[end+1:], "\n"))
	if body == "" {
		return PolicyModule{}, fmt.Errorf("%s: common prompt body is required", path)
	}

	return PolicyModule{
		ID:          id,
		Description: description,
		Default:     defaultOn,
		Order:       order,
		Common:      body,
		Overlays:    make(map[config.Platform]string),
	}, nil
}

// LoadPolicyModules discovers direct module directories, validates their
// metadata, loads optional platform overlays, and returns catalog order.
func LoadPolicyModules(srcDir string) ([]PolicyModule, error) {
	modulesDir := filepath.Join(srcDir, "modules")
	entries, err := os.ReadDir(modulesDir)
	if err != nil {
		return nil, err
	}
	modules := make([]PolicyModule, 0, len(entries))
	seen := make(map[string]bool)
	overlayFiles := map[config.Platform][]string{
		config.PlatformClaude:      {"claude.md"},
		config.PlatformAntigravity: {"antigravity.md", "gemini.md"},
		config.PlatformCodex:       {"codex.md"},
		config.PlatformOpencode:    {"opencode.md"},
	}
	for _, entry := range entries {
		moduleDir := filepath.Join(modulesDir, entry.Name())
		info, statErr := os.Stat(moduleDir)
		if statErr != nil || !info.IsDir() {
			continue
		}
		modulePath := filepath.Join(moduleDir, "module.md")
		if _, statErr := os.Stat(modulePath); os.IsNotExist(statErr) {
			continue
		}
		module, parseErr := parsePolicyModule(modulePath, entry.Name())
		if parseErr != nil {
			return nil, parseErr
		}
		if seen[module.ID] {
			return nil, fmt.Errorf("duplicate policy module id %q", module.ID)
		}
		seen[module.ID] = true
		for platform, names := range overlayFiles {
			for _, name := range names {
				data, readErr := os.ReadFile(filepath.Join(moduleDir, name))
				if os.IsNotExist(readErr) {
					continue
				}
				if readErr != nil {
					return nil, readErr
				}
				overlay := strings.TrimSpace(string(data))
				if overlay != "" {
					module.Overlays[platform] = overlay
				}
				break
			}
		}
		modules = append(modules, module)
	}
	if len(modules) == 0 {
		return nil, fmt.Errorf("no policy modules found in %s", modulesDir)
	}
	sort.Slice(modules, func(i, j int) bool {
		if modules[i].Order == modules[j].Order {
			return modules[i].ID < modules[j].ID
		}
		return modules[i].Order < modules[j].Order
	})
	return modules, nil
}

// DefaultPolicyModuleIDs returns the default-enabled IDs in catalog order.
func DefaultPolicyModuleIDs(modules []PolicyModule) []string {
	var ids []string
	for _, module := range modules {
		if module.Default {
			ids = append(ids, module.ID)
		}
	}
	return ids
}

// BuildPolicyContent renders selected common prompts in catalog order and,
// when requested, appends the current platform's overlay after each module.
func BuildPolicyContent(modules []PolicyModule, selected []string, platform config.Platform, includeOverlay bool) (string, error) {
	selectedSet := make(map[string]bool, len(selected))
	for _, id := range selected {
		selectedSet[id] = true
	}
	known := make(map[string]bool, len(modules))
	var sections []string
	for _, module := range modules {
		known[module.ID] = true
		if !selectedSet[module.ID] {
			continue
		}
		sections = append(sections, strings.TrimSpace(module.Common))
		if includeOverlay {
			if overlay := strings.TrimSpace(module.Overlays[platform]); overlay != "" {
				sections = append(sections, overlay)
			}
		}
	}
	for id := range selectedSet {
		if !known[id] {
			return "", fmt.Errorf("unknown policy module %q", id)
		}
	}
	if len(sections) == 0 {
		return "", nil
	}
	return strings.Join(sections, "\n\n") + "\n", nil
}
