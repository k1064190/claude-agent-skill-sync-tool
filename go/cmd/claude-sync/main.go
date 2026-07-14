// ABOUTME: Unified CLI for syncing Claude Code skills, agents, commands, and rules via symlinks.
// ABOUTME: Presents scope/type/platform selection, then a tree TUI for interactive item picking.

package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
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
	if _, err := findLeafSkills(srcDir, srcDir, &skills, make(map[string]bool)); err != nil {
		return nil, err
	}
	sort.Strings(skills)
	return skills, nil
}

// isDirEntry reports whether e names a directory, following a symlink to a
// directory so that skills registered in the source tree as links to external
// repos are discovered like regular directories.
func isDirEntry(dir string, e os.DirEntry) bool {
	if e.IsDir() {
		return true
	}
	if e.Type()&os.ModeSymlink == 0 {
		return false
	}
	st, err := os.Stat(filepath.Join(dir, e.Name()))
	return err == nil && st.IsDir()
}

// findLeafSkills recursively collects leaf skill directories. visited holds
// canonical (symlink-resolved) paths already traversed, so a symlink pointing
// back at an ancestor (e.g. skills/self -> skills) terminates instead of
// recursing forever; a directory reachable via two links is collected once.
func findLeafSkills(baseDir, dir string, skills *[]string, visited map[string]bool) (bool, error) {
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false, err
	}
	if visited[real] {
		return false, nil
	}
	visited[real] = true

	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}

	hasSubSkill := false
	for _, e := range entries {
		if !isDirEntry(dir, e) {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") || supportDirs[name] {
			continue
		}
		child := filepath.Join(dir, name)
		childIsSkill, err := findLeafSkills(baseDir, child, skills, visited)
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
// for every *.md file found. Unlike filepath.Walk it follows symlinked
// directories — including srcDir itself — so an item-type directory can be a
// link into an external repo.
func collectMdFiles(srcDir string) ([]string, error) {
	var items []string
	if err := findMdFiles(srcDir, srcDir, &items, make(map[string]bool)); err != nil {
		return nil, err
	}
	sort.Strings(items)
	return items, nil
}

// findMdFiles recursively collects *.md paths relative to baseDir. ancestors
// holds the canonical (symlink-resolved) directories on the current recursion
// path, so a link pointing back at an ancestor terminates instead of recursing
// forever. It is scoped to the path rather than the whole walk on purpose: two
// links to the same directory are distinct, separately selectable items, and
// deduplicating them globally would silently drop the second one.
func findMdFiles(baseDir, dir string, items *[]string, ancestors map[string]bool) error {
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return err
	}
	if ancestors[real] {
		return nil // cycle: this directory is its own ancestor
	}
	ancestors[real] = true
	defer delete(ancestors, real)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if isDirEntry(dir, e) {
			if err := findMdFiles(baseDir, path, items, ancestors); err != nil {
				return err
			}
			continue
		}
		if !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		rel, err := filepath.Rel(baseDir, path)
		if err != nil {
			return err
		}
		*items = append(*items, rel)
	}
	return nil
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

// settingsState reports whether a platform's live settings file already has the
// fragment applied: "applied", "pending" (the merge would change it), "missing"
// (no fragment declared), or "error" (unreadable / malformed JSON).
func settingsState(srcDir, destPath string, platform config.Platform) string {
	fragment, err := os.ReadFile(filepath.Join(srcDir, intsync.SettingsSourceFile(platform)))
	if err != nil {
		if os.IsNotExist(err) {
			return "missing" // no fragment declared for this platform
		}
		return "error" // declared but unreadable — --refresh would fail
	}
	live, err := os.ReadFile(destPath)
	if err != nil && !os.IsNotExist(err) {
		return "error"
	}
	_, changed, err := intsync.MergeSettingsContent(live, fragment)
	if err != nil {
		return "error"
	}
	if changed {
		return "pending"
	}
	return "applied"
}

// runCapture pulls the live value of every fragment-owned settings key back into
// the fragment, so a change made through the agent's own UI (e.g. installing a
// plugin with Claude Code's `/plugin`) is not undone by the next --refresh.
func runCapture(cfg *config.Config, scope config.Scope) {
	srcDir := cfg.SourceDir("settings")
	if _, err := os.Stat(srcDir); err != nil {
		fmt.Fprintf(os.Stderr, "No settings/ directory in %s — nothing to capture.\n", cfg.SourceRoot)
		return
	}

	captured := 0
	for _, p := range config.AllPlatforms() {
		if intsync.SettingsTargetName(p) == "" {
			continue
		}
		destDir := config.PlatformDestDir(p, scope, "settings")
		res, err := intsync.CaptureSettings(srcDir, destDir, p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  capture error [%s]: %v\n", p, err)
			continue
		}
		captured += res.Merged
	}

	if captured == 0 {
		fmt.Println("\nCapture complete. Fragments already match the live settings.")
		return
	}
	fmt.Printf("\nCapture complete. Updated %d fragment(s) — commit them to keep the change.\n", captured)
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

		if itemType == "settings" {
			for _, p := range platforms {
				target := intsync.SettingsTargetName(p)
				if target == "" {
					continue // platform has no managed settings file
				}
				destPath := filepath.Join(config.PlatformDestDir(p, scope, itemType), target)
				fmt.Printf("  %-12s %-8s %s\n", p, settingsState(srcDir, destPath, p), destPath)
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

// refreshTemplates rebuilds existing instruction files from their templates,
// preserving prior selections. Platforms are grouped by their resolved
// instruction-file path (Codex and Opencode share ./AGENTS.md in project
// scope). For each path it:
//   - skips missing files and symlinks (e.g. compatibility aliases), which are
//     not tool-owned regular files;
//   - skips a path reached by multiple platforms whose built contents differ,
//     since refresh cannot tell which platform owns the file;
//   - skips a path already in sync;
//   - backs up the existing file to <path>.bak before overwriting, and aborts
//     that rebuild if the backup cannot be written.
//
// Returns the number of instruction files rebuilt.
func refreshTemplates(srcDir string, scope config.Scope, platforms []config.Platform) int {
	// Group platforms by destination path, tracking the distinct built
	// contents that map there and one platform able to produce each path.
	type group struct {
		contents map[string]bool
		sample   config.Platform
	}
	groups := make(map[string]*group)
	var order []string
	for _, p := range platforms {
		destPath := filepath.Join(config.PlatformDestDir(p, scope, "templates"), intsync.TemplateTargetName(p))
		content, _, cerr := intsync.BuildTemplateContent(srcDir, p)
		if cerr != nil {
			fmt.Fprintf(os.Stderr, "  build error [%s]: %v\n", p, cerr)
			continue
		}
		if content == "" {
			continue // no template source for this platform
		}
		g, ok := groups[destPath]
		if !ok {
			g = &group{contents: make(map[string]bool)}
			groups[destPath] = g
			order = append(order, destPath)
		}
		g.contents[content] = true
		g.sample = p
	}

	rebuilt := 0
	for _, destPath := range order {
		g := groups[destPath]

		info, err := os.Lstat(destPath)
		if err != nil || !info.Mode().IsRegular() {
			continue // missing / symlink alias — preserve as-is
		}
		if len(g.contents) > 1 {
			fmt.Fprintf(os.Stderr, "  skipped %s (ambiguous shared target; multiple platforms)\n", destPath)
			continue
		}

		var newContent string
		for c := range g.contents {
			newContent = c
		}
		cur, rerr := os.ReadFile(destPath)
		if rerr != nil {
			// Regular file (checked above) that cannot be read: refuse to
			// overwrite it without being able to back it up first.
			fmt.Fprintf(os.Stderr, "  skipped %s (cannot read to back up: %v)\n", destPath, rerr)
			continue
		}
		if string(cur) == newContent {
			continue // already in sync
		}
		if len(cur) > 0 {
			if err := os.WriteFile(destPath+".bak", cur, 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "  skipped %s (backup failed: %v)\n", destPath, err)
				continue
			}
			fmt.Printf("  backed up: %s.bak\n", destPath)
		}

		destDir := config.PlatformDestDir(g.sample, scope, "templates")
		res, err := intsync.BuildTemplate(srcDir, destDir, g.sample)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  build error [%s]: %v\n", g.sample, err)
			continue
		}
		rebuilt += res.Built
	}
	return rebuilt
}

// linkedRepos scans the given source directories for symlinks and resolves
// each target to the root of the git repository containing it (the nearest
// ancestor holding a .git entry). Symlinks whose targets live outside any git
// repository are ignored. Results are deduplicated and sorted.
//
// Args:
//
//	dirs ([]string): Absolute paths of source directories to scan.
//
// Returns:
//
//	repos ([]string): Sorted, unique git repository roots.
func linkedRepos(dirs []string) []string {
	seen := make(map[string]bool)
	for _, dir := range dirs {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.Mode()&os.ModeSymlink == 0 {
				return nil
			}
			target, err := filepath.EvalSymlinks(path)
			if err != nil {
				return nil // dangling link
			}
			for p := target; ; {
				if _, err := os.Lstat(filepath.Join(p, ".git")); err == nil {
					seen[p] = true
					break
				}
				parent := filepath.Dir(p)
				if parent == p {
					break // reached filesystem root: not inside a repo
				}
				p = parent
			}
			return nil
		})
	}
	repos := make([]string, 0, len(seen))
	for r := range seen {
		repos = append(repos, r)
	}
	sort.Strings(repos)
	return repos
}

// runUpdate git-pulls every repository referenced by symlinks under the source
// root, so all externally cloned skills/agents/rules update in one command.
// Because the source and platform layers are symlinks, pulled changes are
// immediately live everywhere without re-syncing.
func runUpdate(cfg *config.Config) {
	var dirs []string
	for _, t := range config.ItemTypes {
		d := cfg.SourceDir(t)
		if _, err := os.Stat(d); err == nil {
			dirs = append(dirs, d)
		}
	}

	repos := linkedRepos(dirs)
	if len(repos) == 0 {
		fmt.Println("  No linked repositories found under the source root.")
		return
	}

	failed := 0
	for _, repo := range repos {
		out, err := exec.Command("git", "-C", repo, "pull", "--ff-only").CombinedOutput()
		summary := strings.TrimSpace(string(out))
		if i := strings.IndexByte(summary, '\n'); i >= 0 {
			summary = summary[:i]
		}
		if err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "  FAIL %s: %s\n", repo, summary)
			continue
		}
		fmt.Printf("  %s: %s\n", repo, summary)
	}
	fmt.Printf("\nUpdate complete. Pulled %d repo(s), %d failed.\n", len(repos)-failed, failed)
}

// removeDanglingLinks deletes symlinks under destDir that point into srcDir but
// whose target no longer resolves (the source item was deleted or renamed).
// These are the broken links --status reports; refresh cleans them so the two
// commands stay consistent. Returns the number removed.
func removeDanglingLinks(srcDir, destDir string) int {
	removed := 0
	_ = filepath.Walk(destDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
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
		if _, statErr := os.Stat(path); statErr != nil {
			if rmErr := os.Remove(path); rmErr == nil {
				fmt.Printf("  removed broken: %s\n", path)
				removed++
			}
		}
		return nil
	})
	return removed
}

// runRefresh re-applies the current sync state without prompting: it re-links
// items that are already linked (repairing dangling links), rebuilds
// instruction files that already exist (repairing template drift), and injects
// declared settings fragments. It never adds new items or creates instruction
// files that were not present before, so it preserves the user's earlier
// selections.
func runRefresh(cfg *config.Config, scope config.Scope) {
	platforms := config.AllPlatforms()
	totalLinked, totalRemoved, totalRebuilt, totalMerged := 0, 0, 0, 0

	for _, itemType := range config.ItemTypes {
		srcDir := cfg.SourceDir(itemType)
		if _, err := os.Stat(srcDir); err != nil {
			continue
		}

		if itemType == "templates" {
			totalRebuilt += refreshTemplates(srcDir, scope, platforms)
			continue
		}

		if itemType == "settings" {
			for _, p := range platforms {
				destDir := config.PlatformDestDir(p, scope, itemType)
				res, err := intsync.ApplySettings(srcDir, destDir, p)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  settings error [%s]: %v\n", p, err)
					continue
				}
				totalMerged += res.Merged
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
			totalRemoved += removeDanglingLinks(srcDir, destDir)
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

	fmt.Printf("\nRefresh complete. Re-linked %d item(s), removed %d broken link(s), rebuilt %d template(s), merged %d settings file(s).\n",
		totalLinked, totalRemoved, totalRebuilt, totalMerged)
}

// --- Main ---

func main() {
	// --- Flag parsing (non-interactive maintenance modes) ---
	var doStatus, doRefresh, doUpdate, doCapture bool
	scope := config.ScopeUser
	for _, arg := range os.Args[1:] {
		switch arg {
		case "--status", "-s":
			doStatus = true
		case "--refresh", "-r":
			doRefresh = true
		case "--update", "-u":
			doUpdate = true
		case "--capture", "-c":
			doCapture = true
		case "--project", "-p":
			scope = config.ScopeProject
		case "--help", "-h":
			fmt.Println("Usage: claude-sync [--status|-s] [--refresh|-r] [--update|-u] [--capture|-c] [--project|-p]")
			fmt.Println("  (no flags)   interactive sync")
			fmt.Println("  --status     report drift across platforms without changing anything")
			fmt.Println("  --refresh    re-link existing items, rebuild templates, inject settings fragments")
			fmt.Println("  --update     git-pull every repo referenced by symlinks in the source root")
			fmt.Println("  --capture    pull live settings back into the fragments (run after /plugin)")
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
		if doStatus || doRefresh || doUpdate || doCapture {
			fmt.Fprintln(os.Stderr, "No config found. Run claude-sync once interactively first.")
			os.Exit(1)
		}
		cfg, err = config.RunSetup()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Setup error: %v\n", err)
			os.Exit(1)
		}
	}

	if doUpdate {
		runUpdate(cfg)
		// --update composes with --status/--refresh in one invocation.
		if !doStatus && !doRefresh {
			os.Exit(0)
		}
	}
	// Capture runs before refresh so `--capture --refresh` folds live changes
	// into the fragments first, rather than having refresh undo them.
	if doCapture {
		runCapture(cfg, scope)
		if !doStatus && !doRefresh {
			os.Exit(0)
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

	// --- Settings Merge Bypass ---
	// Settings are injected, not symlinked, so there is no tree to pick from.
	if itemType == "settings" {
		fmt.Printf("\nMerging settings fragments into selected platforms...\n")

		totalMerged := 0
		for _, p := range platforms {
			if intsync.SettingsTargetName(p) == "" {
				fmt.Printf("  skipped %s (no managed settings file)\n", p)
				continue
			}
			destDir := config.PlatformDestDir(p, scope, itemType)
			res, err := intsync.ApplySettings(srcDir, destDir, p)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  settings error [%s]: %v\n", p, err)
				continue
			}
			if res.Merged == 0 {
				fmt.Printf("  %s: already in sync\n", p)
			}
			totalMerged += res.Merged
		}

		fmt.Printf("\nDone. Merged %d settings file(s) across %d platform(s)\n", totalMerged, len(platforms))
		os.Exit(0)
	}

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
