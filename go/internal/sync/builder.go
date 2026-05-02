package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/k1064190/claude-agent-skill-sync-tool/go/internal/config"
)

// BuildTemplate concatenates common.md and platform-specific markdown,
// writing the result to the appropriate destination for the platform.
func BuildTemplate(srcDir string, destDir string, platform config.Platform) (Result, error) {
	var res Result

	commonFile := filepath.Join(srcDir, "common.md")
	
	// Determine platform file and target file name
	var platformFile string
	var targetFileName string
	
	switch platform {
	case config.PlatformClaude:
		platformFile = filepath.Join(srcDir, "claude.md")
		targetFileName = "CLAUDE.md"
	case config.PlatformGemini:
		platformFile = filepath.Join(srcDir, "gemini.md")
		targetFileName = "GEMINI.md"
	case config.PlatformCodex:
		platformFile = filepath.Join(srcDir, "codex.md")
		targetFileName = "AGENTS.md"
	case config.PlatformOpencode:
		platformFile = filepath.Join(srcDir, "opencode.md") // Default for opencode, could also use oh-my-opencode
		targetFileName = "AGENTS.md"
	default:
		return res, fmt.Errorf("unknown platform: %s", platform)
	}

	destPath := filepath.Join(destDir, targetFileName)

	var builder strings.Builder

	// Read common.md
	if data, err := os.ReadFile(commonFile); err == nil {
		builder.Write(data)
		builder.WriteString("\n\n")
	}

	// Read platform-specific .md
	if data, err := os.ReadFile(platformFile); err == nil {
		builder.Write(data)
		builder.WriteString("\n")
	}

	// Write to destination
	if builder.Len() > 0 {
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return res, fmt.Errorf("mkdir -p %s: %w", filepath.Dir(destPath), err)
		}
		if err := os.WriteFile(destPath, []byte(builder.String()), 0o644); err != nil {
			return res, fmt.Errorf("write %s: %w", destPath, err)
		}
		absDest, _ := filepath.Abs(destPath)
		fmt.Printf("  built: %s\n", absDest)
		res.Linked++ // Re-using Linked to indicate success
	} else {
		fmt.Printf("  skipped %s (no templates found)\n", targetFileName)
	}

	return res, nil
}
