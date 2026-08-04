package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/k1064190/claude-agent-skill-sync-tool/go/internal/config"
)

func TestWriteProjectPolicyOutputsUsesCanonicalAgentsFile(t *testing.T) {
	root := t.TempDir()
	count, err := writeProjectPolicyOutputs(root, []config.Platform{config.PlatformClaude, config.PlatformCodex}, "# Shared\n", true)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("changed files = %d; want 2", count)
	}
	agents, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(agents) != "# Shared\n" {
		t.Fatalf("AGENTS.md = %q", agents)
	}
	claudePath := filepath.Join(root, "CLAUDE.md")
	info, err := os.Lstat(claudePath)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("CLAUDE.md mode = %s; want regular file", info.Mode())
	}
	claude, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(claude) != "@AGENTS.md\n" {
		t.Fatalf("CLAUDE.md = %q", claude)
	}
}

func TestWriteProjectPolicyOutputsMigratesLegacyAgentsAlias(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("legacy policy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("CLAUDE.md", filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}

	_, err := writeProjectPolicyOutputs(root, []config.Platform{config.PlatformClaude}, "new policy\n", true)
	if err != nil {
		t.Fatalf("writeProjectPolicyOutputs: %v", err)
	}
	agentsInfo, err := os.Lstat(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !agentsInfo.Mode().IsRegular() {
		t.Fatal("AGENTS.md is not a regular canonical file")
	}
	if got, _ := os.ReadFile(filepath.Join(root, "AGENTS.md")); string(got) != "new policy\n" {
		t.Errorf("AGENTS.md = %q; want new policy", got)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md")); string(got) != "@AGENTS.md\n" {
		t.Errorf("CLAUDE.md = %q; want @AGENTS.md reference", got)
	}
}

func TestWritePolicyRegularFileCreatesParentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "AGENTS.md")
	wrote, err := writePolicyRegularFile(path, []byte("policy\n"), true, false)
	if err != nil {
		t.Fatalf("writePolicyRegularFile: %v", err)
	}
	if !wrote {
		t.Fatal("wrote = false; want true")
	}
}

func TestRefreshTemplatesReportsInvalidModularManifest(t *testing.T) {
	root := t.TempDir()
	srcDir := filepath.Join(root, "templates")
	moduleDir := filepath.Join(srcDir, "modules", "interaction")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	module := "---\nid: interaction\ndescription: Interaction rules.\ndefault: true\norder: 10\n---\n# Interaction\n"
	if err := os.WriteFile(filepath.Join(moduleDir, "module.md"), []byte(module), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".claude-sync"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".claude-sync", "policy.toml"), []byte("invalid\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldCWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldCWD) })

	if _, err := refreshTemplates(srcDir, config.ScopeProject, []config.Platform{config.PlatformCodex}); err == nil {
		t.Fatal("refreshTemplates returned nil error for invalid modular manifest")
	}
}

func TestRefreshTemplatesWithoutManifestLeavesExistingPolicyUntouched(t *testing.T) {
	root := t.TempDir()
	srcDir := filepath.Join(root, "templates")
	moduleDir := filepath.Join(srcDir, "modules", "interaction")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	module := "---\nid: interaction\ndescription: Interaction rules.\ndefault: true\norder: 10\n---\n# New policy\n"
	if err := os.WriteFile(filepath.Join(moduleDir, "module.md"), []byte(module), 0o644); err != nil {
		t.Fatal(err)
	}
	policyPath := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(policyPath, []byte("existing policy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldCWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldCWD) })

	rebuilt, err := refreshTemplates(srcDir, config.ScopeProject, []config.Platform{config.PlatformCodex})
	if err != nil {
		t.Fatalf("refreshTemplates: %v", err)
	}
	if rebuilt != 0 {
		t.Fatalf("rebuilt = %d; want 0", rebuilt)
	}
	got, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "existing policy\n" {
		t.Fatalf("AGENTS.md = %q; existing policy was overwritten", got)
	}
}

func TestWriteProjectPolicyOutputsRefreshDoesNotCreateMissingFiles(t *testing.T) {
	root := t.TempDir()
	count, err := writeProjectPolicyOutputs(root, []config.Platform{config.PlatformClaude, config.PlatformCodex}, "# Shared\n", false)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("changed files = %d; want 0", count)
	}
	if _, err := os.Lstat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("AGENTS.md unexpectedly exists: %v", err)
	}
}

// TestCollectSkillsFollowsSymlinks verifies that a skill registered in the
// source tree as a symlink to a directory elsewhere (e.g. a git clone under
// ~/projects) is discovered alongside regular skill directories.
func TestCollectSkillsFollowsSymlinks(t *testing.T) {
	tempDir := t.TempDir()

	// A real skill directory inside the source tree.
	srcDir := filepath.Join(tempDir, "skills")
	if err := os.MkdirAll(filepath.Join(srcDir, "real-skill"), 0o755); err != nil {
		t.Fatal(err)
	}

	// A skill living in an external repo, linked into the source tree.
	repoSkill := filepath.Join(tempDir, "repo", "skills", "linked-skill")
	if err := os.MkdirAll(repoSkill, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(repoSkill, filepath.Join(srcDir, "linked-skill")); err != nil {
		t.Fatal(err)
	}

	skills, err := collectSkills(srcDir)
	if err != nil {
		t.Fatalf("collectSkills: %v", err)
	}

	want := []string{"linked-skill", "real-skill"}
	if len(skills) != len(want) {
		t.Fatalf("collectSkills = %v; want %v", skills, want)
	}
	for i := range want {
		if skills[i] != want[i] {
			t.Errorf("collectSkills[%d] = %q; want %q", i, skills[i], want[i])
		}
	}
}

// TestCollectSkillsSymlinkCycle verifies that a symlink pointing back at an
// ancestor directory does not cause unbounded recursion; discovery must
// terminate and still return the real skills.
func TestCollectSkillsSymlinkCycle(t *testing.T) {
	tempDir := t.TempDir()

	srcDir := filepath.Join(tempDir, "skills")
	if err := os.MkdirAll(filepath.Join(srcDir, "real-skill"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Self-referential cycle: skills/self -> skills.
	if err := os.Symlink(srcDir, filepath.Join(srcDir, "self")); err != nil {
		t.Fatal(err)
	}

	skills, err := collectSkills(srcDir)
	if err != nil {
		t.Fatalf("collectSkills: %v", err)
	}
	if len(skills) != 1 || skills[0] != "real-skill" {
		t.Errorf("collectSkills = %v; want [real-skill]", skills)
	}
}

// TestCollectMdFilesFollowsSymlinks verifies that agents/rules discovery works
// when the item-type directory itself is a symlink into an external repo, and
// when individual entries inside it are symlinks.
func TestCollectMdFilesFollowsSymlinks(t *testing.T) {
	tempDir := t.TempDir()

	// External repo holding the real .md files, including a nested category.
	repo := filepath.Join(tempDir, "repo", "agents")
	if err := os.MkdirAll(filepath.Join(repo, "development"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"top.md", filepath.Join("development", "nested.md")} {
		if err := os.WriteFile(filepath.Join(repo, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Source root's agents/ is a symlink to the repo directory.
	srcRoot := filepath.Join(tempDir, "src")
	if err := os.MkdirAll(srcRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	srcDir := filepath.Join(srcRoot, "agents")
	if err := os.Symlink(repo, srcDir); err != nil {
		t.Fatal(err)
	}

	items, err := collectMdFiles(srcDir)
	if err != nil {
		t.Fatalf("collectMdFiles: %v", err)
	}

	want := []string{filepath.Join("development", "nested.md"), "top.md"}
	if len(items) != len(want) {
		t.Fatalf("collectMdFiles = %v; want %v", items, want)
	}
	for i := range want {
		if items[i] != want[i] {
			t.Errorf("collectMdFiles[%d] = %q; want %q", i, items[i], want[i])
		}
	}
}

// TestCollectMdFilesAliasedSymlinks verifies that two symlinks pointing at the
// same external directory both yield their items: cycle protection must only
// bound the ancestor chain, not deduplicate directories reached by different
// paths (which are distinct, separately selectable sync items).
func TestCollectMdFilesAliasedSymlinks(t *testing.T) {
	tempDir := t.TempDir()

	external := filepath.Join(tempDir, "external")
	if err := os.MkdirAll(external, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(external, "a.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	srcDir := filepath.Join(tempDir, "agents")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one", "two"} {
		if err := os.Symlink(external, filepath.Join(srcDir, name)); err != nil {
			t.Fatal(err)
		}
	}

	items, err := collectMdFiles(srcDir)
	if err != nil {
		t.Fatalf("collectMdFiles: %v", err)
	}

	want := []string{filepath.Join("one", "a.md"), filepath.Join("two", "a.md")}
	if len(items) != len(want) {
		t.Fatalf("collectMdFiles = %v; want %v", items, want)
	}
	for i := range want {
		if items[i] != want[i] {
			t.Errorf("collectMdFiles[%d] = %q; want %q", i, items[i], want[i])
		}
	}
}

// TestCollectMdFilesSymlinkCycle verifies that a symlink pointing back at an
// ancestor does not cause unbounded recursion in .md discovery.
func TestCollectMdFilesSymlinkCycle(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "rules")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "a.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(srcDir, filepath.Join(srcDir, "self")); err != nil {
		t.Fatal(err)
	}

	items, err := collectMdFiles(srcDir)
	if err != nil {
		t.Fatalf("collectMdFiles: %v", err)
	}
	if len(items) != 1 || items[0] != "a.md" {
		t.Errorf("collectMdFiles = %v; want [a.md]", items)
	}
}

// TestCollectCodexAgentFiles verifies that native Codex custom-agent
// discovery includes TOML definitions and excludes Markdown persona agents.
func TestCollectCodexAgentFiles(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "codex-agents")
	if err := os.MkdirAll(filepath.Join(srcDir, "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "review", "adversarial.toml"), []byte("name = \"reviewer\""), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "persona.md"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}

	items, err := collectCodexAgentFiles(srcDir)
	if err != nil {
		t.Fatalf("collectCodexAgentFiles: %v", err)
	}
	want := []string{filepath.Join("review", "adversarial.toml")}
	if len(items) != len(want) || items[0] != want[0] {
		t.Errorf("collectCodexAgentFiles = %v; want %v", items, want)
	}
}

func TestCollectNotifierFilesIncludesOnlyExecutables(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "notifiers")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "codex-notify"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "README.md"), []byte("docs"), 0o644); err != nil {
		t.Fatal(err)
	}

	items, err := collectNotifierFiles(srcDir)
	if err != nil {
		t.Fatalf("collectNotifierFiles: %v", err)
	}
	want := []string{"codex-notify"}
	if len(items) != len(want) || items[0] != want[0] {
		t.Errorf("collectNotifierFiles = %v; want %v", items, want)
	}
}

func TestMigrateLegacyNotifierLinkRemovesOnlyToolOwnedLink(t *testing.T) {
	sourceRoot := t.TempDir()
	destDir := t.TempDir()
	legacyTarget := filepath.Join(sourceRoot, "codex-notifiers", "codex-notify")
	legacyDest := filepath.Join(destDir, "codex-notify")
	if err := os.Symlink(legacyTarget, legacyDest); err != nil {
		t.Fatal(err)
	}

	removed, err := migrateLegacyNotifierLink(sourceRoot, destDir)
	if err != nil {
		t.Fatalf("migrateLegacyNotifierLink: %v", err)
	}
	if !removed {
		t.Fatal("removed = false; want true for exact tool-owned legacy link")
	}
	if _, err := os.Lstat(legacyDest); !os.IsNotExist(err) {
		t.Fatalf("legacy link still exists: %v", err)
	}

	if err := os.Symlink("/opt/foreign/codex-notify", legacyDest); err != nil {
		t.Fatal(err)
	}
	removed, err = migrateLegacyNotifierLink(sourceRoot, destDir)
	if err != nil {
		t.Fatalf("migrateLegacyNotifierLink foreign: %v", err)
	}
	if removed {
		t.Fatal("removed foreign notifier link")
	}
}

func TestClobberSafeNotifierSelectionProtectsRealCommands(t *testing.T) {
	srcDir := t.TempDir()
	destDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(destDir, "codex-notify"), []byte("existing"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/opt/other-notify", filepath.Join(destDir, "foreign-notify")); err != nil {
		t.Fatal(err)
	}

	selected := map[string]bool{"codex-notify": true, "foreign-notify": true, "other-notify": true}
	safe := clobberSafeNotifierSelection(selected, srcDir, destDir)

	if safe["codex-notify"] {
		t.Error("codex-notify marked safe; an existing real command must never be overwritten")
	}
	if !safe["other-notify"] {
		t.Error("other-notify is absent and should be safe to link")
	}
	if safe["foreign-notify"] {
		t.Error("foreign-notify marked safe; a symlink owned by another tool must not be overwritten")
	}
}

// TestLinkedRepos verifies that symlinks in the source tree are resolved to
// the git repository roots containing their targets, deduplicated.
func TestLinkedRepos(t *testing.T) {
	tempDir := t.TempDir()

	// Fake git repo with two skills.
	repo := filepath.Join(tempDir, "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"skill-a", "skill-b"} {
		if err := os.MkdirAll(filepath.Join(repo, "skills", s), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Source tree: two symlinks into the same repo, one plain dir, one
	// symlink to a non-repo location.
	srcDir := filepath.Join(tempDir, "src", "skills")
	if err := os.MkdirAll(filepath.Join(srcDir, "plain"), 0o755); err != nil {
		t.Fatal(err)
	}
	nonRepo := filepath.Join(tempDir, "loose-skill")
	if err := os.MkdirAll(nonRepo, 0o755); err != nil {
		t.Fatal(err)
	}
	links := map[string]string{
		"skill-a": filepath.Join(repo, "skills", "skill-a"),
		"skill-b": filepath.Join(repo, "skills", "skill-b"),
		"loose":   nonRepo,
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(srcDir, name)); err != nil {
			t.Fatal(err)
		}
	}

	repos := linkedRepos([]string{srcDir})

	if len(repos) != 1 || repos[0] != repo {
		t.Errorf("linkedRepos = %v; want [%s]", repos, repo)
	}
}
