package sync

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyncItemsAsFlattensDestination(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	item := filepath.Join("group", "my-skill")
	if err := os.MkdirAll(filepath.Join(src, item), 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := SyncItemsAs([]string{item}, map[string]bool{item: true}, src, dest, filepath.Base)
	if err != nil {
		t.Fatalf("SyncItemsAs: %v", err)
	}
	if res.Linked != 1 {
		t.Errorf("Linked = %d; want 1", res.Linked)
	}
	target, err := os.Readlink(filepath.Join(dest, "my-skill"))
	if err != nil || target != filepath.Join(src, item) {
		t.Errorf("flat link target = %q, %v; want %q", target, err, filepath.Join(src, item))
	}

	// Deselecting removes the flat link.
	res, err = SyncItemsAs([]string{item}, map[string]bool{}, src, dest, filepath.Base)
	if err != nil {
		t.Fatalf("SyncItemsAs deselect: %v", err)
	}
	if res.Removed != 1 {
		t.Errorf("Removed = %d; want 1", res.Removed)
	}
}

func TestCheckDestNameCollisions(t *testing.T) {
	items := []string{filepath.Join("a", "dup"), filepath.Join("b", "dup")}
	if err := CheckDestNameCollisions(items, filepath.Base); err == nil {
		t.Error("expected collision error for duplicate basenames")
	}
	if err := CheckDestNameCollisions(items, IdentityDestName); err != nil {
		t.Errorf("identity mapping must not collide: %v", err)
	}
}

func TestMigrateNestedLinks(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	owned := filepath.Join("group", "owned")
	foreign := filepath.Join("group", "foreign")
	for _, item := range []string{owned, foreign} {
		if err := os.MkdirAll(filepath.Join(src, item), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dest, "group"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(src, owned), filepath.Join(dest, owned)); err != nil {
		t.Fatal(err)
	}

	migrated, err := MigrateNestedLinks([]string{owned}, src, dest, filepath.Base)
	if err != nil {
		t.Fatalf("MigrateNestedLinks: %v", err)
	}
	if !migrated[owned] || len(migrated) != 1 {
		t.Errorf("migrated = %v; want only %q", migrated, owned)
	}
	if _, err := os.Lstat(filepath.Join(dest, owned)); !os.IsNotExist(err) {
		t.Errorf("legacy link still present: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "group")); !os.IsNotExist(err) {
		t.Errorf("empty legacy dir not pruned: %v", err)
	}

	// A link that points elsewhere is not owned by this source and is kept.
	if err := os.MkdirAll(filepath.Join(dest, "group"), 0o755); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	if err := os.Symlink(other, filepath.Join(dest, foreign)); err != nil {
		t.Fatal(err)
	}
	migrated, err = MigrateNestedLinks([]string{foreign}, src, dest, filepath.Base)
	if err != nil {
		t.Fatalf("MigrateNestedLinks foreign: %v", err)
	}
	if len(migrated) != 0 {
		t.Errorf("foreign link migrated: %v", migrated)
	}
	if _, err := os.Lstat(filepath.Join(dest, foreign)); err != nil {
		t.Errorf("foreign link removed: %v", err)
	}
}
