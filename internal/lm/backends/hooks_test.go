package backends

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/benjaminabbitt/scm/internal/config"
)

func TestComputeHookHash(t *testing.T) {
	h1 := config.Hook{Command: "./test.sh", Matcher: "Bash"}
	h2 := config.Hook{Command: "./test.sh", Matcher: "Bash"}
	h3 := config.Hook{Command: "./other.sh", Matcher: "Bash"}

	hash1 := computeHookHash(h1)
	hash2 := computeHookHash(h2)
	hash3 := computeHookHash(h3)

	if hash1 != hash2 {
		t.Errorf("same hooks should have same hash: %s != %s", hash1, hash2)
	}
	if hash1 == hash3 {
		t.Error("different hooks should have different hashes")
	}
	if len(hash1) != 16 {
		t.Errorf("expected 16 char hash, got %d", len(hash1))
	}
}

func TestClaudeCodeHookWriter_WriteHooks(t *testing.T) {
	tmpDir := t.TempDir()
	writer := &ClaudeCodeHookWriter{}

	cfg := &config.HooksConfig{
		Unified: config.UnifiedHooks{
			PreTool: []config.Hook{
				{Command: "./pre-tool.sh", Matcher: "Bash"},
			},
			PostTool: []config.Hook{
				{Command: "./post-tool.sh", Matcher: "Edit"},
			},
		},
	}

	err := writer.WriteHooks(cfg, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify file was created
	settingsPath := filepath.Join(tmpDir, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings.json: %v", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("failed to parse settings.json: %v", err)
	}

	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		t.Fatal("expected hooks in settings")
	}

	// Check PreToolUse
	preToolUse, ok := hooks["PreToolUse"].([]interface{})
	if !ok || len(preToolUse) != 1 {
		t.Errorf("expected 1 PreToolUse matcher, got %v", hooks["PreToolUse"])
	}

	// Check PostToolUse
	postToolUse, ok := hooks["PostToolUse"].([]interface{})
	if !ok || len(postToolUse) != 1 {
		t.Errorf("expected 1 PostToolUse matcher, got %v", hooks["PostToolUse"])
	}
}

func TestClaudeCodeHookWriter_PreservesUserHooks(t *testing.T) {
	tmpDir := t.TempDir()
	writer := &ClaudeCodeHookWriter{}

	// Create existing settings with user hooks (no _scm field)
	claudeDir := filepath.Join(tmpDir, ".claude")
	os.MkdirAll(claudeDir, 0755)

	existingSettings := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"matcher": "Bash",
					"hooks": []interface{}{
						map[string]interface{}{
							"type":    "command",
							"command": "./user-hook.sh",
							// No _scm field - user-defined
						},
					},
				},
			},
		},
		"otherSetting": "preserved",
	}
	data, _ := json.Marshal(existingSettings)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), data, 0644)

	// Write SCM hooks
	cfg := &config.HooksConfig{
		Unified: config.UnifiedHooks{
			PreTool: []config.Hook{
				{Command: "./scm-hook.sh", Matcher: "Bash"},
			},
		},
	}

	err := writer.WriteHooks(cfg, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Read back and verify
	settingsPath := filepath.Join(tmpDir, ".claude", "settings.json")
	data, _ = os.ReadFile(settingsPath)

	var settings map[string]interface{}
	json.Unmarshal(data, &settings)

	// otherSetting should be preserved
	if settings["otherSetting"] != "preserved" {
		t.Error("expected otherSetting to be preserved")
	}

	// Both user hook and SCM hook should exist
	hooks := settings["hooks"].(map[string]interface{})
	preToolUse := hooks["PreToolUse"].([]interface{})

	// Should have 2 matchers (one for user, one for SCM) or combined
	totalHooks := 0
	for _, matcher := range preToolUse {
		m := matcher.(map[string]interface{})
		hooksList := m["hooks"].([]interface{})
		totalHooks += len(hooksList)
	}

	if totalHooks < 2 {
		t.Errorf("expected at least 2 hooks (user + SCM), got %d", totalHooks)
	}
}

func TestClaudeCodeHookWriter_RemovesOldScmHooks(t *testing.T) {
	tmpDir := t.TempDir()
	writer := &ClaudeCodeHookWriter{}

	// Create existing settings with SCM hooks (_scm field present)
	claudeDir := filepath.Join(tmpDir, ".claude")
	os.MkdirAll(claudeDir, 0755)

	existingSettings := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"matcher": "Bash",
					"hooks": []interface{}{
						map[string]interface{}{
							"type":    "command",
							"command": "./old-scm-hook.sh",
							"_scm":    "oldhash123", // SCM-managed
						},
					},
				},
			},
		},
	}
	data, _ := json.Marshal(existingSettings)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), data, 0644)

	// Write new SCM hooks
	cfg := &config.HooksConfig{
		Unified: config.UnifiedHooks{
			PreTool: []config.Hook{
				{Command: "./new-scm-hook.sh", Matcher: "Edit"},
			},
		},
	}

	err := writer.WriteHooks(cfg, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Read back and verify old SCM hook is gone
	settingsPath := filepath.Join(tmpDir, ".claude", "settings.json")
	data, _ = os.ReadFile(settingsPath)

	var settings map[string]interface{}
	json.Unmarshal(data, &settings)

	hooks := settings["hooks"].(map[string]interface{})
	preToolUse := hooks["PreToolUse"].([]interface{})

	// Should only have the new SCM hook with Edit matcher
	for _, matcher := range preToolUse {
		m := matcher.(map[string]interface{})
		if m["matcher"] == "Bash" {
			hooksList := m["hooks"].([]interface{})
			for _, h := range hooksList {
				hook := h.(map[string]interface{})
				if hook["command"] == "./old-scm-hook.sh" {
					t.Error("old SCM hook should have been removed")
				}
			}
		}
	}
}

func TestClaudeCodeHookWriter_UnifiedToBackendMapping(t *testing.T) {
	tmpDir := t.TempDir()
	writer := &ClaudeCodeHookWriter{}

	cfg := &config.HooksConfig{
		Unified: config.UnifiedHooks{
			PreShell:     []config.Hook{{Command: "./pre-shell.sh"}},
			PostFileEdit: []config.Hook{{Command: "./post-edit.sh"}},
			SessionStart: []config.Hook{{Command: "./start.sh"}},
			SessionEnd:   []config.Hook{{Command: "./end.sh"}},
		},
	}

	err := writer.WriteHooks(cfg, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	settingsPath := filepath.Join(tmpDir, ".claude", "settings.json")
	data, _ := os.ReadFile(settingsPath)

	var settings map[string]interface{}
	json.Unmarshal(data, &settings)

	hooks := settings["hooks"].(map[string]interface{})

	// PreShell maps to PreToolUse with Bash matcher
	preToolUse := hooks["PreToolUse"].([]interface{})
	foundBashMatcher := false
	for _, m := range preToolUse {
		matcher := m.(map[string]interface{})
		if matcher["matcher"] == "Bash" {
			foundBashMatcher = true
		}
	}
	if !foundBashMatcher {
		t.Error("PreShell should map to PreToolUse with Bash matcher")
	}

	// PostFileEdit maps to PostToolUse with Edit|Write matcher
	postToolUse := hooks["PostToolUse"].([]interface{})
	foundEditMatcher := false
	for _, m := range postToolUse {
		matcher := m.(map[string]interface{})
		if matcher["matcher"] == "Edit|Write" {
			foundEditMatcher = true
		}
	}
	if !foundEditMatcher {
		t.Error("PostFileEdit should map to PostToolUse with Edit|Write matcher")
	}

	// SessionStart and SessionEnd should be present
	if _, ok := hooks["SessionStart"]; !ok {
		t.Error("expected SessionStart hook")
	}
	if _, ok := hooks["SessionEnd"]; !ok {
		t.Error("expected SessionEnd hook")
	}
}

func TestClaudeCodeHookWriter_BackendPassthrough(t *testing.T) {
	tmpDir := t.TempDir()
	writer := &ClaudeCodeHookWriter{}

	cfg := &config.HooksConfig{
		Plugins: map[string]config.BackendHooks{
			"claude-code": {
				"Notification": []config.Hook{
					{Command: "./notify.sh", Type: "command"},
				},
				"PreCompact": []config.Hook{
					{Command: "./compact.sh"},
				},
			},
		},
	}

	err := writer.WriteHooks(cfg, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	settingsPath := filepath.Join(tmpDir, ".claude", "settings.json")
	data, _ := os.ReadFile(settingsPath)

	var settings map[string]interface{}
	json.Unmarshal(data, &settings)

	hooks := settings["hooks"].(map[string]interface{})

	if _, ok := hooks["Notification"]; !ok {
		t.Error("expected Notification hook from passthrough")
	}
	if _, ok := hooks["PreCompact"]; !ok {
		t.Error("expected PreCompact hook from passthrough")
	}
}

func TestGetHookWriter(t *testing.T) {
	if GetHookWriter("claude-code") == nil {
		t.Error("expected hook writer for claude-code")
	}
	if GetHookWriter("unknown-backend") != nil {
		t.Error("expected nil for unknown backend")
	}
}

func TestNewContextInjectionHook(t *testing.T) {
	hook := NewContextInjectionHook()

	if hook.Command != ContextInjectionCommand {
		t.Errorf("expected command %q, got %q", ContextInjectionCommand, hook.Command)
	}
	if hook.Type != "command" {
		t.Errorf("expected type 'command', got %q", hook.Type)
	}
	if hook.Timeout != ContextInjectionTimeout {
		t.Errorf("expected timeout %d, got %d", ContextInjectionTimeout, hook.Timeout)
	}
	if hook.Timeout == 0 {
		t.Error("timeout should not be zero - prevents Claude from hanging if scm isn't found")
	}
}
