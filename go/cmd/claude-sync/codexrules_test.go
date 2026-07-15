package main

import (
	"os"
	"path/filepath"
	"testing"
)

// clobberSafeRules must never mark a rule whose destination is a real
// (non-symlink) file — those belong to Codex or a guard and must not be deleted.
func TestClobberSafeRulesProtectsRealFiles(t *testing.T) {
	dest := t.TempDir()
	src := t.TempDir()

	// A real file already in the rules dir (e.g. Codex's default.rules) plus a
	// pre-existing symlink this tool owns.
	if err := os.WriteFile(filepath.Join(dest, "default.rules"), []byte("real"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(src, "ours.rules"), filepath.Join(dest, "ours.rules")); err != nil {
		t.Fatal(err)
	}

	safe := clobberSafeRules([]string{"default.rules", "ours.rules", "new.rules"}, dest)

	if safe["default.rules"] {
		t.Error("default.rules marked safe; a real file must never be overwritten")
	}
	if !safe["ours.rules"] {
		t.Error("ours.rules (our own symlink) should be safe to re-link")
	}
	if !safe["new.rules"] {
		t.Error("new.rules (absent) should be safe to link")
	}

	// The real file must still be intact and a regular file.
	info, err := os.Lstat(filepath.Join(dest, "default.rules"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("the real default.rules was replaced by a symlink")
	}
}
