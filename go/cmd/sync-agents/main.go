// ABOUTME: CLI entry point for syncing Claude agent .md files to ~/.claude/agents/ via symlinks.
// ABOUTME: Walks claude/agents/ for *.md files, presents a tree TUI, then applies symlink changes.

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

	// Fallback: use the directory of the executable.
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

// collectAgents walks srcDir recursively and returns a sorted list of relative
// paths for every *.md file found (e.g. "business/pm.md").
//
// Args:
//
//	srcDir (string): Absolute path to the agents source directory.
//
// Returns:
//
//	agents ([]string): Sorted relative paths.
//	err    (error):    Walk error, or nil on success.
func collectAgents(srcDir string) ([]string, error) {
	var agents []string

	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".md") {
			rel, relErr := filepath.Rel(srcDir, path)
			if relErr != nil {
				return relErr
			}
			agents = append(agents, rel)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(agents)
	return agents, nil
}

func main() {
	root := projectRoot()
	agentsSrc := filepath.Join(root, "claude", "agents")
	agentsDest := filepath.Join(targetHome(), ".claude", "agents")

	fmt.Println("=== Claude Agent Sync ===")
	fmt.Printf("Source : %s\n", agentsSrc)
	fmt.Printf("Dest   : %s\n", agentsDest)
	fmt.Println()

	allAgents, err := collectAgents(agentsSrc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning agents: %v\n", err)
		os.Exit(1)
	}
	if len(allAgents) == 0 {
		fmt.Fprintf(os.Stderr, "No agents found in %s\n", agentsSrc)
		os.Exit(1)
	}

	// Build description callback: reads from the .md file directly.
	descFn := func(relPath string) string {
		return yaml.ExtractDescription(filepath.Join(agentsSrc, relPath))
	}

	m := tree.NewModel(allAgents, descFn)
	p := tea.NewProgram(m, tea.WithOutput(os.Stderr))

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
		fmt.Println("No agents selected. Exiting.")
		os.Exit(0)
	}

	fmt.Printf("\nSelected %d agent(s):\n", len(result.SelectedPaths))
	for _, a := range result.SelectedPaths {
		fmt.Printf("  - %s\n", a)
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

	if err := os.MkdirAll(agentsDest, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Cannot create dest dir: %v\n", err)
		os.Exit(1)
	}

	// Build selected set for O(1) lookup.
	selectedSet := make(map[string]bool, len(result.SelectedPaths))
	for _, p := range result.SelectedPaths {
		selectedSet[p] = true
	}

	syncResult, err := intsync.SyncItems(allAgents, selectedSet, agentsSrc, agentsDest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Sync error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nDone. Linked %d, removed %d agent(s) in %s\n",
		syncResult.Linked, syncResult.Removed, agentsDest)
}
