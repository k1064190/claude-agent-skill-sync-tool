package config

import (
	"os"
	"path/filepath"
)

// Platform represents an AI agent platform.
type Platform string

const (
	PlatformClaude   Platform = "Claude"
	PlatformGemini   Platform = "Gemini"
	PlatformCodex    Platform = "Codex"
	PlatformOpencode Platform = "Opencode"
)

// AllPlatforms returns a list of all supported platforms.
func AllPlatforms() []Platform {
	return []Platform{PlatformClaude, PlatformGemini, PlatformCodex, PlatformOpencode}
}

// PlatformDestDir returns the destination directory for the given platform, scope, and item type.
// If the target is project scope, it prefixes with ./ (except Opencode which might be global only, but let's follow the standard).
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

	if itemType == "templates" {
		itemType = ""
	}

	var dir string
	switch platform {
	case PlatformClaude:
		dir = filepath.Join(base, ".claude", itemType)
	case PlatformGemini:
		dir = filepath.Join(base, ".gemini", itemType)
	case PlatformCodex:
		dir = filepath.Join(base, ".codex", itemType)
	case PlatformOpencode:
		if scope == ScopeProject {
			dir = filepath.Join(base, ".config", "opencode", itemType)
		} else {
			dir = filepath.Join(base, ".config", "opencode", itemType)
		}
	}
	return filepath.Clean(dir)
}
