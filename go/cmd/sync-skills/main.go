// ABOUTME: CLI entry point for syncing Claude skill directories to ~/.claude/skills/ via symlinks.
// ABOUTME: Walks claude/skills/ for SKILL.md files, uses their parent dirs as items, presents tree TUI.

package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	intsync "github.com/k1064190/claude-agent-skill-sync-tool/go/internal/sync"
	"github.com/k1064190/claude-agent-skill-sync-tool/go/internal/tree"
	"github.com/k1064190/claude-agent-skill-sync-tool/go/internal/yaml"
)

// projectRoot locates the repository root by walking up from the executable's
// directory until it finds a directory that contains a "claude" subdirectory,
// or falls back to the executable's own directory when no such parent exists.
//
// Returns:
//
//	root (string): Absolute path to the project root directory.
func projectRoot() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	dir := filepath.Dir(exe)

	// Walk up the directory tree looking for the claude/ subdirectory.
	for {
		if _, err := os.Stat(filepath.Join(dir, "claude")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return filepath.Dir(exe)
}

// targetHome returns the destination home directory, preferring the
// SYNC_TARGET_HOME environment variable and falling back to os.UserHomeDir.
//
// Returns:
//
//	home (string): Absolute path to the home directory to use for dest.
func targetHome() string {
	if v := os.Getenv("SYNC_TARGET_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return os.Getenv("HOME")
	}
	return home
}

// collectSkills walks srcDir recursively looking for files named "SKILL.md".
// The parent directory of each SKILL.md (relative to srcDir) is the skill item.
// Results are sorted lexicographically.
//
// Args:
//
//	srcDir (string): Absolute path to the skills source directory.
//
// Returns:
//
//	skills ([]string): Sorted relative paths of skill directories (e.g. "somecat/my-skill").
//	err    (error):    Walk error, or nil on success.
func collectSkills(srcDir string) ([]string, error) {
	var skills []string

	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && info.Name() == "SKILL.md" {
			// The skill item is the directory containing SKILL.md.
			skillDir := filepath.Dir(path)
			rel, relErr := filepath.Rel(srcDir, skillDir)
			if relErr != nil {
				return relErr
			}
			skills = append(skills, rel)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(skills)
	return skills, nil
}

func main() {
	root := projectRoot()
	skillsSrc := filepath.Join(root, "claude", "skills")
	skillsDest := filepath.Join(targetHome(), ".claude", "skills")

	fmt.Println("=== Claude Skill Sync ===")
	fmt.Printf("Source : %s\n", skillsSrc)
	fmt.Printf("Dest   : %s\n", skillsDest)
	fmt.Println()

	allSkills, err := collectSkills(skillsSrc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning skills: %v\n", err)
		os.Exit(1)
	}
	if len(allSkills) == 0 {
		fmt.Fprintf(os.Stderr, "No skills found in %s\n", skillsSrc)
		os.Exit(1)
	}

	// Build description callback: reads from the SKILL.md inside the skill directory.
	descFn := func(relPath string) string {
		return yaml.ExtractDescription(filepath.Join(skillsSrc, relPath, "SKILL.md"))
	}

	m := tree.NewModel(allSkills, descFn)
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
		fmt.Println("No skills selected. Exiting.")
		os.Exit(0)
	}

	fmt.Printf("\nSelected %d skill(s):\n", len(result.SelectedPaths))
	for _, s := range result.SelectedPaths {
		fmt.Printf("  - %s\n", s)
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
