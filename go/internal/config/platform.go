package config

import (
	"os"
	"path/filepath"
)

// Platform represents an AI agent platform.
type Platform string

const (
	PlatformClaude      Platform = "Claude"
	PlatformAntigravity Platform = "Antigravity"
	PlatformCodex       Platform = "Codex"
	PlatformOpencode    Platform = "Opencode"
)

// AllPlatforms returns a list of all supported platforms.
func AllPlatforms() []Platform {
	return []Platform{PlatformClaude, PlatformAntigravity, PlatformCodex, PlatformOpencode}
}

// PlatformDestDir returns the destination directory for the given platform, scope, and item type.
// It returns an absolute path using the user's home directory or the current working directory.
func PlatformDestDir(platform Platform, scope Scope, itemType string) string {
	var base string
	switch scope {
	case ScopeProject:
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "."
		}
		base = cwd
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			home = os.Getenv("HOME")
		}
		base = home
	}

	isTemplate := false
	if itemType == "templates" {
		itemType = ""
		isTemplate = true
	}

	// Settings live directly in the platform's config directory (e.g.
	// ~/.claude/settings.json, ./.claude/settings.json), so the destination is
	// that directory itself — not a per-item-type subdirectory.
	if itemType == "settings" {
		itemType = ""
	}

	// Codex execpolicy rules are Codex-only and user-global: Codex enforces them
	// only from ~/.codex/rules/, never a project-local dir, so this ignores scope
	// and always targets the home directory. Other platforms have no equivalent
	// and get an empty destination (the caller skips them).
	if itemType == "codex-rules" {
		if platform != PlatformCodex {
			return ""
		}
		home, err := os.UserHomeDir()
		if err != nil {
			home = os.Getenv("HOME")
		}
		return filepath.Clean(filepath.Join(home, ".codex", "rules"))
	}

	// For Project Scope templates, the destination is the project root itself
	// (e.g. ./CLAUDE.md, ./GEMINI.md, ./AGENTS.md)
	if scope == ScopeProject && isTemplate {
		return filepath.Clean(base)
	}

	var dir string
	switch platform {
	case PlatformClaude:
		dir = filepath.Join(base, ".claude", itemType)
	case PlatformAntigravity:
		// Antigravity (agy) discovery locations: the workspace customization
		// root is .agents/ (project scope), and the global customization root
		// is ~/.gemini/config/ (user scope).
		switch {
		case isTemplate:
			// User-scope instruction file → ~/.gemini/GEMINI.md.
			// (Project-scope templates already returned at the project root.)
			dir = filepath.Join(base, ".gemini")
		case scope == ScopeProject:
			dir = filepath.Join(base, ".agents", itemType)
		default:
			dir = filepath.Join(base, ".gemini", "config", itemType)
		}
	case PlatformCodex:
		if !isTemplate && itemType == "skills" {
			// Codex standardizes on .agents/ directory for skills
			dir = filepath.Join(base, ".agents", itemType)
		} else {
			dir = filepath.Join(base, ".codex", itemType)
		}
	case PlatformOpencode:
		if scope == ScopeProject {
			dir = filepath.Join(base, ".config", "opencode", itemType)
		} else {
			dir = filepath.Join(base, ".config", "opencode", itemType)
		}
	default:
		// Unknown platform - return empty string to indicate failure or handled elsewhere
		return ""
	}
	return filepath.Clean(dir)
}
