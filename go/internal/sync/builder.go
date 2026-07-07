package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/k1064190/claude-agent-skill-sync-tool/go/internal/config"
)

// TemplateTargetName returns the instruction file name written for a platform
// (e.g. "CLAUDE.md"). Returns "" for an unknown platform.
func TemplateTargetName(platform config.Platform) string {
	switch platform {
	case config.PlatformClaude:
		return "CLAUDE.md"
	case config.PlatformAntigravity:
		return "GEMINI.md"
	case config.PlatformCodex, config.PlatformOpencode:
		return "AGENTS.md"
	default:
		return ""
	}
}

// templateSourceFile returns the platform-specific override markdown file name
// under the templates source directory.
func templateSourceFile(platform config.Platform) string {
	switch platform {
	case config.PlatformClaude:
		return "claude.md"
	case config.PlatformAntigravity:
		// Antigravity (agy) reads GEMINI.md; the source override still lives in
		// templates/gemini.md.
		return "gemini.md"
	case config.PlatformCodex:
		return "codex.md"
	case config.PlatformOpencode:
		return "opencode.md"
	default:
		return ""
	}
}

// BuildTemplateContent concatenates common.md and the platform-specific
// markdown override, returning the merged content without writing it. It is the
// pure core shared by BuildTemplate (which writes) and status reporting (which
// only compares).
//
// Returns:
//
//	content    (string): Merged template text (empty if no source found).
//	targetName (string): Destination file name (e.g. "CLAUDE.md").
//	err        (error):  Read error, or nil.
func BuildTemplateContent(srcDir string, platform config.Platform) (string, string, error) {
	targetName := TemplateTargetName(platform)
	if targetName == "" {
		return "", "", fmt.Errorf("unknown platform: %s", platform)
	}

	commonFile := filepath.Join(srcDir, "common.md")
	platformFile := filepath.Join(srcDir, templateSourceFile(platform))

	var builder strings.Builder

	if data, err := os.ReadFile(commonFile); err == nil {
		builder.Write(data)
		builder.WriteString("\n\n")
	} else if !os.IsNotExist(err) {
		return "", targetName, fmt.Errorf("read common template %s: %w", commonFile, err)
	}

	if data, err := os.ReadFile(platformFile); err == nil {
		builder.Write(data)
		builder.WriteString("\n")
	} else if !os.IsNotExist(err) {
		return "", targetName, fmt.Errorf("read platform template %s: %w", platformFile, err)
	}

	return builder.String(), targetName, nil
}

// BuildTemplate concatenates common.md and platform-specific markdown,
// writing the result to the appropriate destination for the platform.
func BuildTemplate(srcDir string, destDir string, platform config.Platform) (Result, error) {
	var res Result

	content, targetFileName, err := BuildTemplateContent(srcDir, platform)
	if err != nil {
		return res, err
	}

	destPath := filepath.Join(destDir, targetFileName)

	// Write to destination
	if len(content) > 0 {
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return res, fmt.Errorf("mkdir -p %s: %w", filepath.Dir(destPath), err)
		}
		if err := os.WriteFile(destPath, []byte(content), 0o644); err != nil {
			return res, fmt.Errorf("write %s: %w", destPath, err)
		}

		displayPath := destPath
		if abs, err := filepath.Abs(destPath); err == nil {
			displayPath = abs
		}
		fmt.Printf("  built: %s\n", displayPath)
		res.Built++ // Successfully built
	} else {
		fmt.Printf("  skipped %s (no templates found)\n", targetFileName)
	}

	return res, nil
}
