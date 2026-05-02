package config

import (
	"path/filepath"
	"testing"
)

func TestPlatformDestDir(t *testing.T) {
	// Let's just check the suffix since base dir can vary by user/cwd
	tests := []struct {
		platform Platform
		scope    Scope
		itemType string
		suffix   string
	}{
		{PlatformClaude, ScopeUser, "skills", filepath.Join(".claude", "skills")},
		{PlatformGemini, ScopeUser, "skills", filepath.Join(".agents", "skills")},
		{PlatformCodex, ScopeProject, "agents", filepath.Join(".codex", "agents")},
		{PlatformOpencode, ScopeUser, "templates", filepath.Join(".config", "opencode")},
		{PlatformClaude, ScopeProject, "templates", ".claude"},
	}

	for _, tt := range tests {
		result := PlatformDestDir(tt.platform, tt.scope, tt.itemType)
		if filepath.Base(result) != filepath.Base(tt.suffix) {
			// Basic heuristic check to ensure the end of the path matches
			t.Errorf("PlatformDestDir(%v, %v, %v) = %v; expected to end with %v",
				tt.platform, tt.scope, tt.itemType, result, tt.suffix)
		}
	}
}
