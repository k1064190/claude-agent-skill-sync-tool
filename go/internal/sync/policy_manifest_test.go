package sync

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPolicyManifestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude-sync", "policy.toml")
	want := PolicyManifest{Version: 1, Modules: []string{"interaction", "git-workflow"}}
	if err := WritePolicyManifest(path, want); err != nil {
		t.Fatal(err)
	}
	got, exists, err := ReadPolicyManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if !exists || !reflect.DeepEqual(got, want) {
		t.Fatalf("manifest = %#v, exists=%v; want %#v", got, exists, want)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	wantText := "version = 1\nmodules = [\"interaction\", \"git-workflow\"]\n"
	if string(data) != wantText {
		t.Fatalf("manifest text = %q; want %q", data, wantText)
	}
}

func TestReadPolicyManifestMissing(t *testing.T) {
	_, exists, err := ReadPolicyManifest(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil || exists {
		t.Fatalf("exists=%v err=%v; want false, nil", exists, err)
	}
}

func TestReadPolicyManifestRejectsDuplicateModules(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.toml")
	if err := os.WriteFile(path, []byte("version = 1\nmodules = [\"testing\", \"testing\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadPolicyManifest(path); err == nil {
		t.Fatal("expected duplicate module error")
	}
}
