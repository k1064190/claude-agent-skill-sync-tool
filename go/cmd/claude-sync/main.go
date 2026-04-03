// ABOUTME: Unified CLI for syncing Claude Code skills, agents, commands, and rules via symlinks.
// ABOUTME: Presents scope/type selection, then a tree TUI for interactive item picking.

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

// --- Skill discovery (leaf directory detection) ---

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

// collectSkills discovers leaf skill directories under srcDir.
func collectSkills(srcDir string) ([]string, error) {
	var skills []string
	if _, err := findLeafSkills(srcDir, srcDir, &skills); err != nil {
		return nil, err
	}
	sort.Strings(skills)
	return skills, nil
}

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

// --- .md file discovery (agents, commands, rules) ---

// collectMdFiles walks srcDir recursively and returns sorted relative paths
// for every *.md file found.
func collectMdFiles(srcDir string) ([]string, error) {
	var items []string

	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".md") {
			rel, relErr := filepath.Rel(srcDir, path)
			if relErr != nil {
				return relErr
			}
			items = append(items, rel)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(items)
	return items, nil
}

// --- Main ---

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
	fmt.Println()

	// --- Item Type Selection ---
	itemType := config.SelectItemType(cfg)

	srcDir := cfg.SourceDir(itemType)
	destDir := config.DestDir(scope, itemType)

	fmt.Printf("  Source : %s\n", srcDir)
	fmt.Printf("  Dest   : %s\n\n", destDir)

	// --- Discover items ---
	var allItems []string
	if itemType == "skills" {
		allItems, err = collectSkills(srcDir)
	} else {
		allItems, err = collectMdFiles(srcDir)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning %s: %v\n", itemType, err)
		os.Exit(1)
	}
	if len(allItems) == 0 {
		fmt.Fprintf(os.Stderr, "No %s found in %s\n", itemType, srcDir)
		os.Exit(1)
	}

	// --- Determine initial selection from existing symlinks ---
	existing := config.ExistingSymlinks(allItems, srcDir, destDir)

	// --- Build description callback ---
	var descFn tree.DescFunc
	switch itemType {
	case "skills":
		descFn = func(relPath string) string {
			return yaml.ExtractDescription(filepath.Join(srcDir, relPath, "SKILL.md"))
		}
	case "agents", "rules":
		descFn = func(relPath string) string {
			return yaml.ExtractDescription(filepath.Join(srcDir, relPath))
		}
	}

	// --- TUI ---
	m := tree.NewModel(allItems, descFn, existing)
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
		fmt.Printf("\nNo %s selected — existing symlinks will be removed.\n", itemType)
	} else {
		fmt.Printf("\nSelected %d %s:\n", len(result.SelectedPaths), itemType)
		for _, s := range result.SelectedPaths {
			fmt.Printf("  - %s\n", s)
		}
	}

	// --- Confirmation ---
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

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Cannot create dest dir: %v\n", err)
		os.Exit(1)
	}

	selectedSet := make(map[string]bool, len(result.SelectedPaths))
	for _, p := range result.SelectedPaths {
		selectedSet[p] = true
	}

	syncResult, err := intsync.SyncItems(allItems, selectedSet, srcDir, destDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Sync error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nDone. Linked %d, removed %d %s in %s\n",
		syncResult.Linked, syncResult.Removed, itemType, destDir)
}
