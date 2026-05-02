package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/k1064190/claude-agent-skill-sync-tool/go/internal/config"
)

func TestBuildTemplate(t *testing.T) {
	// Setup temp dirs
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	destDir := filepath.Join(tempDir, "dest")
	
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write common.md
	if err := os.WriteFile(filepath.Join(srcDir, "common.md"), []byte("common content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Write claude.md
	if err := os.WriteFile(filepath.Join(srcDir, "claude.md"), []byte("claude content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Test Claude platform
	res, err := BuildTemplate(srcDir, destDir, config.PlatformClaude)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res.Built != 1 {
		t.Errorf("expected 1 built, got %d", res.Built)
	}

	// Verify CLAUDE.md content
	content, err := os.ReadFile(filepath.Join(destDir, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}

	expected := "common content\n\nclaude content\n"
	if string(content) != expected {
		t.Errorf("expected %q, got %q", expected, string(content))
	}
}
