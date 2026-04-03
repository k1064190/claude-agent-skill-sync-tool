// ABOUTME: Manages persistent configuration for claude-sync tools.
// ABOUTME: Handles config load/save, first-run setup, scope selection, and ASCII title display.

package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Scope represents the destination scope for symlink operations.
type Scope int

const (
	// ScopeUser targets ~/.claude/ (user-wide).
	ScopeUser Scope = iota
	// ScopeProject targets ./.claude/ (current working directory).
	ScopeProject
)

// Config holds persistent settings saved to disk.
type Config struct {
	// SourceRoot is the absolute path to the directory containing
	// skills/ and agents/ subdirectories.
	SourceRoot string `json:"source_root"`
}

// SkillsSource returns the absolute path to the skills source directory.
func (c *Config) SkillsSource() string {
	return filepath.Join(c.SourceRoot, "skills")
}

// AgentsSource returns the absolute path to the agents source directory.
func (c *Config) AgentsSource() string {
	return filepath.Join(c.SourceRoot, "agents")
}

// configDir returns the path to ~/.config/claude-sync/.
func configDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".config", "claude-sync")
}

// configPath returns the full path to the config file.
func configPath() string {
	return filepath.Join(configDir(), "config.json")
}

// Load reads the config file from disk. Returns nil if it does not exist.
//
// Returns:
//
//	cfg (*Config): Loaded config, or nil if no config file found.
//	err (error):   Read/parse error, or nil.
func Load() (*Config, error) {
	data, err := os.ReadFile(configPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Save writes the config to disk, creating the directory if needed.
//
// Args:
//
//	cfg (*Config): Config to persist.
//
// Returns:
//
//	err (error): Write error, or nil.
func Save(cfg *Config) error {
	if err := os.MkdirAll(configDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), data, 0o644)
}

const title = `
  ___ _      _   _   _ ___  ___   _____   ___  _  ___
 / __| |    /_\ | | | |   \| __| / __\ \ / / \| |/ __|
| (__| |__ / _ \| |_| | |) | _|  \__ \\ V /| .  | (__
 \___|____/_/ \_\\___/|___/|___| |___/ |_| |_|\_|\___|
`

// PrintTitle prints the ASCII art title banner.
func PrintTitle() {
	fmt.Print(title)
}

// readLine reads a single trimmed line from /dev/tty.
func readLine() string {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		// Fallback to stdin.
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		return strings.TrimSpace(scanner.Text())
	}
	defer tty.Close()
	scanner := bufio.NewScanner(tty)
	scanner.Scan()
	return strings.TrimSpace(scanner.Text())
}

// RunSetup guides the user through first-time configuration. It detects the
// source directory from the binary location (projectRoot) and asks for
// confirmation.
//
// Args:
//
//	projectRoot (string): Detected project root containing claude/ directory.
//
// Returns:
//
//	cfg (*Config): Newly created config.
//	err (error):   Save error, or nil.
func RunSetup() (*Config, error) {
	fmt.Println("  Setting up configuration...\n")

	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}

	fmt.Printf("  Source root [%s]:\n  > ", cwd)
	input := readLine()
	if input == "" {
		input = cwd
	}

	// Resolve to absolute path.
	abs, err := filepath.Abs(input)
	if err == nil {
		input = abs
	}

	// Verify skills/ or agents/ exists under the source root.
	skillsDir := filepath.Join(input, "skills")
	agentsDir := filepath.Join(input, "agents")
	_, sErr := os.Stat(skillsDir)
	_, aErr := os.Stat(agentsDir)
	if sErr != nil && aErr != nil {
		fmt.Printf("  Warning: neither %s nor %s found.\n", skillsDir, agentsDir)
		fmt.Print("  Continue anyway? [y/N]: ")
		answer := readLine()
		if answer != "y" && answer != "Y" {
			fmt.Println("  Setup cancelled.")
			os.Exit(0)
		}
	}

	cfg := &Config{SourceRoot: input}

	if err := Save(cfg); err != nil {
		return nil, fmt.Errorf("save config: %w", err)
	}
	fmt.Printf("\n  Config saved to %s\n", configPath())
	fmt.Printf("  Skills : %s\n", cfg.SkillsSource())
	fmt.Printf("  Agents : %s\n\n", cfg.AgentsSource())
	return cfg, nil
}

// SelectScope displays the scope selection menu and returns the user's choice.
// If the user selects "Set configuration", it re-runs setup and returns the
// chosen scope on retry.
//
// Args:
//
//	projectRoot (string): Detected project root (for re-running setup).
//
// Returns:
//
//	scope (Scope):   Selected scope.
//	cfg   (*Config): Possibly updated config (if user reconfigured).
//	err   (error):   Error, or nil.
func SelectScope(cfg *Config) (Scope, *Config, error) {
	for {
		fmt.Println("  [1] User scope    (~/.claude/)")
		fmt.Println("  [2] Project scope (./.claude/)")
		fmt.Println("  [3] Set configuration")
		fmt.Print("\n  Select [1/2/3]: ")

		answer := readLine()
		switch answer {
		case "1", "":
			return ScopeUser, cfg, nil
		case "2":
			return ScopeProject, cfg, nil
		case "3":
			newCfg, err := RunSetup()
			if err != nil {
				return ScopeUser, cfg, err
			}
			cfg = newCfg
			continue
		default:
			fmt.Println("  Invalid choice. Please enter 1, 2, or 3.\n")
		}
	}
}

// DestDir returns the destination directory for the given scope and item type.
//
// Args:
//
//	scope    (Scope):  User or Project scope.
//	itemType (string): "skills" or "agents".
//
// Returns:
//
//	dest (string): Absolute path to the destination directory.
func DestDir(scope Scope, itemType string) string {
	switch scope {
	case ScopeProject:
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "."
		}
		return filepath.Join(cwd, ".claude", itemType)
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			home = os.Getenv("HOME")
		}
		return filepath.Join(home, ".claude", itemType)
	}
}

// ExistingSymlinks scans destDir and returns relative paths that are symlinks
// pointing into srcDir. These represent previously synced items.
//
// Args:
//
//	allItems ([]string): All discovered relative item paths.
//	srcDir   (string):   Absolute path to the source directory.
//	destDir  (string):   Absolute path to the destination directory.
//
// Returns:
//
//	existing (map[string]bool): Set of relative paths with active symlinks.
func ExistingSymlinks(allItems []string, srcDir, destDir string) map[string]bool {
	existing := make(map[string]bool)
	for _, item := range allItems {
		dest := filepath.Join(destDir, item)
		target, err := os.Readlink(dest)
		if err != nil {
			continue
		}
		expected := filepath.Join(srcDir, item)
		if target == expected {
			existing[item] = true
		}
	}
	return existing
}
