package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlatformDestDir(t *testing.T) {
	// Let's just check the suffix since base dir can vary by user/cwd
	tests := []struct {
		platform Platform
		scope    Scope
		itemType string
		expected string // We will check if it ends with this
	}{
		{PlatformClaude, ScopeUser, "skills", filepath.Join(".claude", "skills")},
		// Antigravity user scope → global customization root ~/.gemini/config/.
		{PlatformAntigravity, ScopeUser, "skills", filepath.Join(".gemini", "config", "skills")},
		{PlatformAntigravity, ScopeUser, "rules", filepath.Join(".gemini", "config", "rules")},
		{PlatformAntigravity, ScopeUser, "templates", ".gemini"},
		// Antigravity project scope → workspace customization root .agents/.
		{PlatformAntigravity, ScopeProject, "skills", filepath.Join(".agents", "skills")},
		{PlatformAntigravity, ScopeProject, "rules", filepath.Join(".agents", "rules")},
		{PlatformCodex, ScopeProject, "agents", filepath.Join(".codex", "agents")},
		{PlatformOpencode, ScopeUser, "templates", filepath.Join(".config", "opencode")},
		{PlatformClaude, ScopeProject, "templates", "."},      // Project root
		{PlatformAntigravity, ScopeProject, "templates", "."}, // Project root
		{PlatformCodex, ScopeProject, "templates", "."},       // Project root
	}

	for _, tt := range tests {
		result := PlatformDestDir(tt.platform, tt.scope, tt.itemType)
		// Clean the expected path
		expected := tt.expected
		if expected == "." {
			cwd, _ := os.Getwd()
			if result != cwd {
				t.Errorf("PlatformDestDir(%v, %v, %v) = %v; expected project root %v",
					tt.platform, tt.scope, tt.itemType, result, cwd)
			}
			continue
		}

		if !strings.HasSuffix(result, expected) {
			t.Errorf("PlatformDestDir(%v, %v, %v) = %v; expected to end with %v",
				tt.platform, tt.scope, tt.itemType, result, expected)
		}
	}
}
