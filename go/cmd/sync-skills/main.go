// ABOUTME: CLI entry point for syncing Claude skill directories via symlinks.
// ABOUTME: Discovers leaf skill directories, presents scope selection and tree TUI.

package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/k1064190/claude-agent-skill-sync-tool/go/internal/config"
	intsync "github.com/k1064190/claude-agent-skill-sync-tool/go/internal/sync"
	"github.com/k1064190/claude-agent-skill-sync-tool/go/internal/tree"
	"github.com/k1064190/claude-agent-skill-sync-tool/go/internal/yaml"
)

// supportDirs lists directory names that belong to a skill's internal
// structure and should not be treated as sub-skill directories.
var supportDirs = map[string]bool{
	"references": true, "templates": true, "scripts": true,
	"docs": true, "dev_data": true, "examples": true,
	"demos": true, "packages": true, "anthropic_official_docs": true,
	"node_modules": true, "__pycache__": true,
	"template": true, "researcher": true,
	"video-promo": true, "src": true, "public": true,
}

// collectSkills discovers leaf skill directories under srcDir. A directory is
// a leaf skill when it contains no sub-directories that are themselves skills
// (support directories like references/ and templates/ are excluded from this
// check). Results are sorted lexicographically.
//
// Args:
//
//	srcDir (string): Absolute path to the skills source directory.
//
// Returns:
//
//	skills ([]string): Sorted relative paths of leaf skill directories.
//	err    (error):    ReadDir error, or nil on success.
func collectSkills(srcDir string) ([]string, error) {
	var skills []string
	if _, err := findLeafSkills(srcDir, srcDir, &skills); err != nil {
		return nil, err
	}
	sort.Strings(skills)
	return skills, nil
}

// findLeafSkills recursively walks dir and appends relative paths of leaf
// skill directories to skills. Returns true if dir is or contains a skill
// directory (so the caller knows not to treat itself as a leaf).
//
// Args:
//
//	baseDir (string):    The root skills directory (for computing relative paths).
//	dir     (string):    The current directory being inspected.
//	skills  (*[]string): Accumulator for discovered leaf skill paths.
//
// Returns:
//
//	isSkill (bool): True if dir is or contains at least one skill.
//	err     (error): First ReadDir error encountered, or nil.
func findLeafSkills(baseDir, dir string, skills *[]string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}

	hasSubSkill := false
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") || supportDirs[name] {
			continue
		}
		child := filepath.Join(dir, name)
		childIsSkill, err := findLeafSkills(baseDir, child, skills)
		if err != nil {
			return false, err
		}
		if childIsSkill {
			hasSubSkill = true
		}
	}

	// A leaf skill: has no sub-skill children and is not the base directory.
	if !hasSubSkill && dir != baseDir {
		rel, err := filepath.Rel(baseDir, dir)
		if err != nil {
			return false, err
		}
		*skills = append(*skills, rel)
		return true, nil
	}

	return hasSubSkill, nil
}

func main() {
	// --- Title & Config ---
	config.PrintTitle()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if cfg == nil {
		cfg, err = config.RunSetup()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Setup error: %v\n", err)
			os.Exit(1)
		}
	}

	// --- Scope Selection ---
	scope, cfg, err := config.SelectScope(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	skillsSrc := cfg.SkillsSource()
	skillsDest := config.DestDir(scope, "skills")

	fmt.Printf("\n  Source : %s\n", skillsSrc)
	fmt.Printf("  Dest   : %s\n\n", skillsDest)

	// --- Discover skills ---
	allSkills, err := collectSkills(skillsSrc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning skills: %v\n", err)
		os.Exit(1)
	}
	if len(allSkills) == 0 {
		fmt.Fprintf(os.Stderr, "No skills found in %s\n", skillsSrc)
		os.Exit(1)
	}

	// --- Determine initial selection from existing symlinks ---
	existing := config.ExistingSymlinks(allSkills, skillsSrc, skillsDest)

	// Build description callback.
	descFn := func(relPath string) string {
		return yaml.ExtractDescription(filepath.Join(skillsSrc, relPath, "SKILL.md"))
	}

	m := tree.NewModel(allSkills, descFn, existing)
	p := tea.NewProgram(m, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		os.Exit(1)
	}

	result, ok := finalModel.(tree.Model)
	if !ok {
		fmt.Fprintln(os.Stderr, "Internal error: unexpected model type")
		os.Exit(1)
	}

	if !result.Confirmed {
		fmt.Println("Selection cancelled.")
		os.Exit(0)
	}

	if len(result.SelectedPaths) == 0 {
		fmt.Println("\nNo skills selected — existing symlinks will be removed.")
	} else {
		fmt.Printf("\nSelected %d skill(s):\n", len(result.SelectedPaths))
		for _, s := range result.SelectedPaths {
			fmt.Printf("  - %s\n", s)
		}
	}

	// Read confirmation from /dev/tty so it works regardless of stdin state.
	fmt.Println()
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot open /dev/tty: %v\n", err)
		os.Exit(1)
	}
	defer tty.Close()

	fmt.Fprint(tty, "Proceed with sync? [y/N]: ")
	scanner := bufio.NewScanner(tty)
	scanner.Scan()
	answer := strings.TrimSpace(scanner.Text())

	if answer != "y" && answer != "Y" {
		fmt.Println("Aborted.")
		os.Exit(0)
	}

	if err := os.MkdirAll(skillsDest, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Cannot create dest dir: %v\n", err)
		os.Exit(1)
	}

	// Build selected set for O(1) lookup.
	selectedSet := make(map[string]bool, len(result.SelectedPaths))
	for _, p := range result.SelectedPaths {
		selectedSet[p] = true
	}

	syncResult, err := intsync.SyncItems(allSkills, selectedSet, skillsSrc, skillsDest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Sync error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nDone. Linked %d, removed %d skill(s) in %s\n",
		syncResult.Linked, syncResult.Removed, skillsDest)
}
