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
