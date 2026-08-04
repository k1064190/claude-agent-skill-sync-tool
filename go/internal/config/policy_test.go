package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPolicyManifestPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	userPath, err := PolicyManifestPath(ScopeUser)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".config", "claude-sync", "policy.toml"); userPath != want {
		t.Fatalf("user manifest = %q; want %q", userPath, want)
	}

	project := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	projectPath, err := PolicyManifestPath(ScopeProject)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(project, ".claude-sync", "policy.toml"); projectPath != want {
		t.Fatalf("project manifest = %q; want %q", projectPath, want)
	}
}
