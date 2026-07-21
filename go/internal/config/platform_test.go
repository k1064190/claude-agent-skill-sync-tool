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
		{PlatformCodex, ScopeUser, "codex-agents", filepath.Join(".codex", "agents")},
		{PlatformCodex, ScopeProject, "codex-agents", filepath.Join(".codex", "agents")},
		{PlatformCodex, ScopeUser, "codex-notifiers", filepath.Join(".local", "bin")},
		{PlatformCodex, ScopeProject, "codex-notifiers", filepath.Join(".local", "bin")},
		// codex-rules is user-global regardless of scope: Codex enforces from
		// ~/.codex/rules/ only, so project scope must NOT point at ./.codex/rules.
		{PlatformCodex, ScopeUser, "codex-rules", filepath.Join(".codex", "rules")},
		{PlatformCodex, ScopeProject, "codex-rules", filepath.Join(".codex", "rules")},
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

func TestPlatformDestDirCodexNotifiersRejectsOtherPlatforms(t *testing.T) {
	for _, platform := range []Platform{PlatformClaude, PlatformAntigravity, PlatformOpencode} {
		if got := PlatformDestDir(platform, ScopeUser, "codex-notifiers"); got != "" {
			t.Errorf("PlatformDestDir(%v, ScopeUser, codex-notifiers) = %q; want empty", platform, got)
		}
	}
}

func TestPlatformDestDirCodexNotifiersRequiresHome(t *testing.T) {
	t.Setenv("HOME", "")
	if got := PlatformDestDir(PlatformCodex, ScopeUser, "codex-notifiers"); got != "" {
		t.Errorf("PlatformDestDir(Codex, ScopeUser, codex-notifiers) = %q; want empty without a home directory", got)
	}
}

func TestPlatformDestDirCodexAgentsRejectsOtherPlatforms(t *testing.T) {
	for _, platform := range []Platform{PlatformClaude, PlatformAntigravity, PlatformOpencode} {
		if got := PlatformDestDir(platform, ScopeUser, "codex-agents"); got != "" {
			t.Errorf("PlatformDestDir(%v, ScopeUser, codex-agents) = %q; want empty", platform, got)
		}
	}
}
