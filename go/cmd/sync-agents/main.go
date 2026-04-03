// ABOUTME: CLI entry point for syncing Claude agent .md files via symlinks.
// ABOUTME: Discovers .md files, presents scope selection and tree TUI.

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

	agentsSrc := cfg.AgentsSource()
	agentsDest := config.DestDir(scope, "agents")

	fmt.Printf("\n  Source : %s\n", agentsSrc)
	fmt.Printf("  Dest   : %s\n\n", agentsDest)

	// --- Discover agents ---
	allAgents, err := collectAgents(agentsSrc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning agents: %v\n", err)
		os.Exit(1)
	}
	if len(allAgents) == 0 {
		fmt.Fprintf(os.Stderr, "No agents found in %s\n", agentsSrc)
		os.Exit(1)
	}

	// --- Determine initial selection from existing symlinks ---
	existing := config.ExistingSymlinks(allAgents, agentsSrc, agentsDest)

	// Build description callback.
	descFn := func(relPath string) string {
		return yaml.ExtractDescription(filepath.Join(agentsSrc, relPath))
	}

	m := tree.NewModel(allAgents, descFn, existing)
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
		fmt.Println("\nNo agents selected — existing symlinks will be removed.")
	} else {
		fmt.Printf("\nSelected %d agent(s):\n", len(result.SelectedPaths))
		for _, a := range result.SelectedPaths {
			fmt.Printf("  - %s\n", a)
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
