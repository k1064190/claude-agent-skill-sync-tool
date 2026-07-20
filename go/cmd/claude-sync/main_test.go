package main

import (
	"os"
	"path/filepath"
	"testing"
)

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
