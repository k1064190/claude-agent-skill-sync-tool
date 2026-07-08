// ABOUTME: Unified CLI for syncing Claude Code skills, agents, commands, and rules via symlinks.
// ABOUTME: Presents scope/type/platform selection, then a tree TUI for interactive item picking.

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
	"github.com/k1064190/claude-agent-skill-sync-tool/go/internal/ui"
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

// --- Non-interactive maintenance modes ---

// scanLinks walks destDir and counts every symlink that points into srcDir,
// whether or not the linked item still exists in the source. Scanning the
// destination (rather than the current source list) is what lets --status
// report links whose source was deleted or renamed as broken.
//
// Returns:
//
//	linked (int): Symlinks into srcDir whose target resolves on disk.
//	broken (int): Symlinks into srcDir whose target is missing (dangling).
func scanLinks(srcDir, destDir string) (linked, broken int) {
	_ = filepath.Walk(destDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil // tolerate unreadable entries; Walk won't descend symlinks
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return nil
		}
		target, err := os.Readlink(path)
		if err != nil {
			return nil
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		rel, err := filepath.Rel(srcDir, target)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return nil // not a link into the sync source
		}
		if _, err := os.Stat(path); err != nil {
			broken++
		} else {
			linked++
		}
		return nil
	})
	return linked, broken
}

// templateState compares freshly built template content against the installed
// instruction file, returning "in-sync", "stale", or "missing".
func templateState(content, destPath string) string {
	data, err := os.ReadFile(destPath)
	if os.IsNotExist(err) {
		return "missing"
	}
	if err != nil {
		return "error"
	}
	if string(data) == content {
		return "in-sync"
	}
	return "stale"
}

// runStatus reports, for every platform and item type, how the destinations
// compare to the source of truth. It never mutates the filesystem.
func runStatus(cfg *config.Config, scope config.Scope) {
	platforms := config.AllPlatforms()
	for _, itemType := range config.ItemTypes {
		srcDir := cfg.SourceDir(itemType)
		if _, err := os.Stat(srcDir); err != nil {
			continue
		}
		fmt.Printf("\n== %s ==\n", itemType)

		if itemType == "templates" {
			for _, p := range platforms {
				content, target, err := intsync.BuildTemplateContent(srcDir, p)
				if err != nil || content == "" {
					continue
				}
				destPath := filepath.Join(config.PlatformDestDir(p, scope, itemType), target)
				fmt.Printf("  %-12s %-8s %s\n", p, templateState(content, destPath), destPath)
			}
			continue
		}

		for _, p := range platforms {
			destDir := config.PlatformDestDir(p, scope, itemType)
			linked, broken := scanLinks(srcDir, destDir)
			fmt.Printf("  %-12s linked=%-3d broken=%-3d %s\n", p, linked, broken, destDir)
		}
	}
}

// runRefresh re-applies the current sync state without prompting: it re-links
// items that are already linked (repairing dangling links) and rebuilds
// instruction files that already exist (repairing template drift). It never
// adds new items or creates instruction files that were not present before, so
// it preserves the user's earlier selections.
func runRefresh(cfg *config.Config, scope config.Scope) {
	platforms := config.AllPlatforms()
	totalLinked, totalRebuilt := 0, 0

	for _, itemType := range config.ItemTypes {
		srcDir := cfg.SourceDir(itemType)
		if _, err := os.Stat(srcDir); err != nil {
			continue
		}

		if itemType == "templates" {
			rebuilt := make(map[string]bool)
			for _, p := range platforms {
				destDir := config.PlatformDestDir(p, scope, itemType)
				destPath := filepath.Join(destDir, intsync.TemplateTargetName(p))
				// Only rebuild an existing, regular instruction file. Skip
				// missing files (preserve the prior selection), skip symlinks
				// such as compatibility aliases (AGENTS.md -> CLAUDE.md), whose
				// target would be clobbered by writing through them, and rebuild
				// each concrete path only once (Codex and Opencode both resolve
				// to ./AGENTS.md in project scope).
				info, err := os.Lstat(destPath)
				if err != nil || !info.Mode().IsRegular() {
					continue
				}
				if rebuilt[destPath] {
					continue
				}
				rebuilt[destPath] = true

				// Skip when already in sync, and back up the existing file
				// before overwriting so a hand-authored instruction file is
				// never silently destroyed.
				newContent, _, cerr := intsync.BuildTemplateContent(srcDir, p)
				cur, _ := os.ReadFile(destPath)
				if cerr == nil && string(cur) == newContent {
					continue
				}
				if len(cur) > 0 {
					if err := os.WriteFile(destPath+".bak", cur, 0o644); err != nil {
						fmt.Fprintf(os.Stderr, "  backup error [%s]: %v\n", p, err)
					} else {
						fmt.Printf("  backed up: %s.bak\n", destPath)
					}
				}

				res, err := intsync.BuildTemplate(srcDir, destDir, p)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  build error [%s]: %v\n", p, err)
					continue
				}
				totalRebuilt += res.Built
			}
			continue
		}

		var allItems []string
		var err error
		if itemType == "skills" {
			allItems, err = collectSkills(srcDir)
		} else {
			allItems, err = collectMdFiles(srcDir)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "  scan error [%s]: %v\n", itemType, err)
			continue
		}
		for _, p := range platforms {
			destDir := config.PlatformDestDir(p, scope, itemType)
			existing := config.ExistingSymlinks(allItems, srcDir, destDir)
			if len(existing) == 0 {
				continue
			}
			res, err := intsync.SyncItems(allItems, existing, srcDir, destDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  relink error [%s/%s]: %v\n", p, itemType, err)
				continue
			}
			totalLinked += res.Linked
		}
	}

	fmt.Printf("\nRefresh complete. Re-linked %d item(s), rebuilt %d template(s).\n", totalLinked, totalRebuilt)
}

// --- Main ---

func main() {
	// --- Flag parsing (non-interactive maintenance modes) ---
	var doStatus, doRefresh bool
	scope := config.ScopeUser
	for _, arg := range os.Args[1:] {
		switch arg {
		case "--status", "-s":
			doStatus = true
		case "--refresh", "-r":
			doRefresh = true
		case "--project", "-p":
			scope = config.ScopeProject
		case "--help", "-h":
			fmt.Println("Usage: claude-sync [--status|-s] [--refresh|-r] [--project|-p]")
			fmt.Println("  (no flags)   interactive sync")
			fmt.Println("  --status     report drift across platforms without changing anything")
			fmt.Println("  --refresh    re-link existing items and rebuild existing templates")
			fmt.Println("  --project    operate on project scope (./) instead of user scope (~/)")
			os.Exit(0)
		}
	}

	// --- Title & Config ---
	config.PrintTitle()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if cfg == nil {
		if doStatus || doRefresh {
			fmt.Fprintln(os.Stderr, "No config found. Run claude-sync once interactively first.")
			os.Exit(1)
		}
		cfg, err = config.RunSetup()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Setup error: %v\n", err)
			os.Exit(1)
		}
	}

	if doStatus {
		runStatus(cfg, scope)
		os.Exit(0)
	}
	if doRefresh {
		runRefresh(cfg, scope)
		os.Exit(0)
	}

	// --- Platform Selection ---
	platforms, err := ui.RunPlatformSelect()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error selecting platforms: %v\n", err)
		os.Exit(1)
	}
	if len(platforms) == 0 {
		fmt.Println("No platforms selected. Cancelled.")
		os.Exit(0)
	}

	// --- Scope Selection ---
	scope, cfg, err = config.SelectScope(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println()

	// --- Item Type Selection ---
	itemType := config.SelectItemType(cfg)

	srcDir := cfg.SourceDir(itemType)

	fmt.Printf("  Source : %s\n", srcDir)
	fmt.Printf("  Targets:\n")
	for _, p := range platforms {
		dest := config.PlatformDestDir(p, scope, itemType)
		displayPath := dest
		if abs, err := filepath.Abs(dest); err == nil {
			displayPath = abs
		}
		fmt.Printf("    - [%s] %s\n", p, displayPath)
	}
	fmt.Println()

	// --- Templates Builder Bypass ---
	if itemType == "templates" {
		fmt.Printf("\nBuilding templates for selected platforms...\n")

		totalBuilt := 0
		for _, p := range platforms {
			destDir := config.PlatformDestDir(p, scope, itemType)
			fmt.Printf("\nBuilding for %s...\n", p)
			syncResult, err := intsync.BuildTemplate(srcDir, destDir, p)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Build error for %s: %v\n", p, err)
			} else {
				totalBuilt += syncResult.Built
			}
		}

		fmt.Printf("\nDone. Built %d templates across %d platform(s)\n", totalBuilt, len(platforms))

		// --- Compatibility Symlinks (Project Scope only) ---
		if scope == config.ScopeProject {
			cwd, _ := os.Getwd()
			files := []string{"CLAUDE.md", "AGENTS.md", "GEMINI.md"}

			// Find existing real files among the set
			var existingReal []string
			for _, f := range files {
				path := filepath.Join(cwd, f)
				info, err := os.Lstat(path)
				if err == nil && !info.Mode().IsRegular() {
					// It's a symlink or something else, skip it for "real" source
					continue
				}
				if err == nil && info.Mode().IsRegular() {
					existingReal = append(existingReal, f)
				}
			}

			if len(existingReal) > 0 {
				// Use the first real file as the source for missing ones
				source := existingReal[0]
				for _, f := range files {
					if f == source {
						continue
					}
					target := filepath.Join(cwd, f)
					// If it doesn't exist at all, create symlink
					if _, err := os.Lstat(target); os.IsNotExist(err) {
						if err := os.Symlink(source, target); err == nil {
							displayPath := target
							if abs, err := filepath.Abs(target); err == nil {
								displayPath = abs
							}
							fmt.Printf("  linked: %s -> %s (compatibility)\n", displayPath, source)
						}
					}
				}
			}
		}

		os.Exit(0)
	}

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
	// Union of existing symlinks across all platforms
	existingUnion := make(map[string]bool)
	for _, p := range platforms {
		destDir := config.PlatformDestDir(p, scope, itemType)
		existing := config.ExistingSymlinks(allItems, srcDir, destDir)
		for k, v := range existing {
			if v {
				existingUnion[k] = true
			}
		}
	}

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
	m := tree.NewModel(allItems, descFn, existingUnion)
	prog := tea.NewProgram(m, tea.WithAltScreen())

	finalModel, err := prog.Run()
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

	selectedSet := make(map[string]bool, len(result.SelectedPaths))
	for _, p := range result.SelectedPaths {
		selectedSet[p] = true
	}

	// Sync to all selected platforms
	totalLinked := 0
	totalRemoved := 0
	for _, p := range platforms {
		destDir := config.PlatformDestDir(p, scope, itemType)
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "Cannot create dest dir %s: %v\n", destDir, err)
			continue
		}

		fmt.Printf("\nSyncing to %s...\n", p)
		syncResult, err := intsync.SyncItems(allItems, selectedSet, srcDir, destDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Sync error for %s: %v\n", p, err)
			// Continue to next platform
		} else {
			totalLinked += syncResult.Linked
			totalRemoved += syncResult.Removed
		}
	}

	fmt.Printf("\nDone. Linked %d, removed %d total %s across %d platform(s)\n",
		totalLinked, totalRemoved, itemType, len(platforms))
}
