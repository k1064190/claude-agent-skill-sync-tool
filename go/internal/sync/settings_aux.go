// ABOUTME: Applies additive hooks and explicit removals alongside owned settings fragments.
// ABOUTME: Keeps shared automation additive without taking ownership of machine-local settings.

package sync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/k1064190/claude-agent-skill-sync-tool/go/internal/config"
)

const claudeHooksSource = "claude-hooks.json"

func readOptionalFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return data, err
}

func jsonFieldIndex(fields []jsonField, key string) int {
	for i, field := range fields {
		if field.Key == key {
			return i
		}
	}
	return -1
}

func decodeRawArray(raw json.RawMessage) ([]json.RawMessage, error) {
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	return values, nil
}

// MergeClaudeHooksContent appends hook groups from a hooks-only fragment to
// the live settings' matching events. Existing events and hook groups retain
// their order and values; semantically identical managed groups are not added
// twice.
//
// Args:
//
//	live ([]byte): Current Claude settings JSON.
//	fragment ([]byte): Hooks-only JSON object keyed by event name.
//
// Returns:
//
//	merged ([]byte): Settings with missing managed hook groups appended.
//	changed (bool): Whether the resulting settings differ semantically.
//	err (error): Malformed settings, hooks, or fragment content.
func MergeClaudeHooksContent(live, fragment []byte) ([]byte, bool, error) {
	liveFields, err := decodeOrderedObject(live)
	if err != nil {
		return nil, false, fmt.Errorf("parse live settings: %w", err)
	}
	fragmentEvents, err := decodeOrderedObject(fragment)
	if err != nil {
		return nil, false, fmt.Errorf("parse Claude hooks fragment: %w", err)
	}

	hooksIndex := jsonFieldIndex(liveFields, "hooks")
	var liveEvents []jsonField
	if hooksIndex >= 0 {
		liveEvents, err = decodeOrderedObject(liveFields[hooksIndex].Val)
		if err != nil {
			return nil, false, fmt.Errorf("parse live hooks: %w", err)
		}
	}

	updated := false
	for _, fragmentEvent := range fragmentEvents {
		managedGroups, err := decodeRawArray(fragmentEvent.Val)
		if err != nil {
			return nil, false, fmt.Errorf("parse Claude %s hook groups: %w", fragmentEvent.Key, err)
		}
		eventIndex := jsonFieldIndex(liveEvents, fragmentEvent.Key)
		var liveGroups []json.RawMessage
		if eventIndex >= 0 {
			liveGroups, err = decodeRawArray(liveEvents[eventIndex].Val)
			if err != nil {
				return nil, false, fmt.Errorf("parse live Claude %s hook groups: %w", fragmentEvent.Key, err)
			}
		}
		for _, managed := range managedGroups {
			found := false
			for _, existing := range liveGroups {
				if equalJSON(existing, managed) {
					found = true
					break
				}
			}
			if !found {
				liveGroups = append(liveGroups, managed)
				updated = true
			}
		}
		if eventIndex < 0 {
			liveEvents = append(liveEvents, jsonField{Key: fragmentEvent.Key})
			eventIndex = len(liveEvents) - 1
		}
		encodedGroups, err := json.Marshal(liveGroups)
		if err != nil {
			return nil, false, fmt.Errorf("encode Claude %s hook groups: %w", fragmentEvent.Key, err)
		}
		liveEvents[eventIndex].Val = encodedGroups
	}

	if !updated {
		return live, false, nil
	}
	encodedHooks, err := encodeOrderedObject(liveEvents)
	if err != nil {
		return nil, false, fmt.Errorf("encode Claude hooks: %w", err)
	}
	hooksValue := json.RawMessage(bytes.TrimSpace(encodedHooks))
	if hooksIndex >= 0 {
		liveFields[hooksIndex].Val = hooksValue
	} else {
		liveFields = append(liveFields, jsonField{Key: "hooks", Val: hooksValue})
	}
	out, err := encodeOrderedObject(liveFields)
	if err != nil {
		return nil, false, fmt.Errorf("encode live settings: %w", err)
	}
	return out, !equalJSON(live, out), nil
}

func readUnsetKeys(path string) ([]string, error) {
	data, err := readOptionalFile(path)
	if err != nil || data == nil {
		return nil, err
	}
	var keys []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		keys = append(keys, line)
	}
	return keys, nil
}

// MergeSettingsFromSource applies a platform's owned base fragment and its
// optional additive/removal companion files to live settings.
//
// Args:
//
//	srcDir (string): Directory containing the platform settings sources.
//	platform (config.Platform): Claude or Codex settings format to apply.
//	live ([]byte): Current live settings content.
//
// Returns:
//
//	merged ([]byte): Fully transformed settings content.
//	changed (bool): Whether live settings need to be rewritten.
//	err (error): Source read or content validation failure.
func MergeSettingsFromSource(srcDir string, platform config.Platform, live []byte) ([]byte, bool, error) {
	fragment, err := os.ReadFile(filepath.Join(srcDir, SettingsSourceFile(platform)))
	if err != nil {
		return nil, false, fmt.Errorf("read settings fragment: %w", err)
	}
	merged, _, err := MergeSettingsForPlatform(platform, live, fragment)
	if err != nil {
		return nil, false, err
	}

	switch platform {
	case config.PlatformClaude:
		hooks, err := readOptionalFile(filepath.Join(srcDir, claudeHooksSource))
		if err != nil {
			return nil, false, fmt.Errorf("read Claude hooks fragment: %w", err)
		}
		if hooks != nil {
			merged, _, err = MergeClaudeHooksContent(merged, hooks)
			if err != nil {
				return nil, false, err
			}
		}
		return merged, !equalJSON(live, merged), nil
	case config.PlatformCodex:
		keys, err := readUnsetKeys(filepath.Join(srcDir, "codex.unset"))
		if err != nil {
			return nil, false, fmt.Errorf("read Codex unset settings: %w", err)
		}
		if len(keys) > 0 {
			merged, _, err = RemoveTomlSettingsKeys(merged, keys)
			if err != nil {
				return nil, false, err
			}
		}
		return merged, !bytes.Equal(live, merged), nil
	default:
		return merged, false, nil
	}
}
