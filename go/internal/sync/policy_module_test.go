package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/k1064190/claude-agent-skill-sync-tool/go/internal/config"
)

func writePolicyModule(t *testing.T, root, id, description string, defaultOn bool, order int, body string) {
	t.Helper()
	dir := filepath.Join(root, "modules", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nid: " + id + "\ndescription: " + description + "\ndefault: "
	if defaultOn {
		content += "true"
	} else {
		content += "false"
	}
	content += fmt.Sprintf("\norder: %d\n---\n\n%s\n", order, body)
	if err := os.WriteFile(filepath.Join(dir, "module.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadPolicyModulesAndBuildContent(t *testing.T) {
	root := t.TempDir()
	writePolicyModule(t, root, "testing", "Run tests", false, 2, "# Testing\n\nTest changed behavior.")
	writePolicyModule(t, root, "interaction", "Interaction rules", true, 1, "# Interaction\n\nState assumptions.")
	if err := os.WriteFile(filepath.Join(root, "modules", "interaction", "codex.md"), []byte("## Codex\n\nUse Codex tools.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	modules, err := LoadPolicyModules(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{modules[0].ID, modules[1].ID}; !reflect.DeepEqual(got, []string{"interaction", "testing"}) {
		t.Fatalf("module order = %v", got)
	}
	if got := DefaultPolicyModuleIDs(modules); !reflect.DeepEqual(got, []string{"interaction"}) {
		t.Fatalf("defaults = %v", got)
	}

	userContent, err := BuildPolicyContent(modules, []string{"testing", "interaction"}, config.PlatformCodex, true)
	if err != nil {
		t.Fatal(err)
	}
	wantUser := "# Interaction\n\nState assumptions.\n\n## Codex\n\nUse Codex tools.\n\n# Testing\n\nTest changed behavior.\n"
	if userContent != wantUser {
		t.Fatalf("user content = %q; want %q", userContent, wantUser)
	}

	projectContent, err := BuildPolicyContent(modules, []string{"testing", "interaction"}, config.PlatformClaude, false)
	if err != nil {
		t.Fatal(err)
	}
	wantProject := "# Interaction\n\nState assumptions.\n\n# Testing\n\nTest changed behavior.\n"
	if projectContent != wantProject {
		t.Fatalf("project content = %q; want %q", projectContent, wantProject)
	}
}

func TestLoadPolicyModulesRejectsDirectoryIDMismatch(t *testing.T) {
	root := t.TempDir()
	writePolicyModule(t, root, "testing", "Run tests", false, 1, "# Testing")
	path := filepath.Join(root, "modules", "testing", "module.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte("---\nid: other\ndescription: Run tests\ndefault: false\norder: 1\n---\n\n# Testing\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPolicyModules(root); err == nil {
		t.Fatal("expected ID mismatch error")
	}
}

func TestBuildPolicyContentRejectsUnknownSelection(t *testing.T) {
	root := t.TempDir()
	writePolicyModule(t, root, "interaction", "Interaction rules", true, 1, "# Interaction")
	modules, err := LoadPolicyModules(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildPolicyContent(modules, []string{"missing"}, config.PlatformCodex, true); err == nil {
		t.Fatal("expected unknown module error")
	}
}
