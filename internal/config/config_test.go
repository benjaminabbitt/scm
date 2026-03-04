package config

import (
	"testing"
)

func TestSetDefaultPlugin_ExistingPlugin(t *testing.T) {
	lm := LMConfig{
		Plugins: map[string]PluginConfig{
			"claude-code": {Default: true},
			"gemini":      {Default: false, Model: "gemini-2.0-flash"},
		},
	}

	lm.SetDefaultPlugin("gemini")

	if lm.Plugins["claude-code"].Default {
		t.Error("expected claude-code Default to be false")
	}
	if !lm.Plugins["gemini"].Default {
		t.Error("expected gemini Default to be true")
	}
	if lm.Plugins["gemini"].Model != "gemini-2.0-flash" {
		t.Error("expected gemini Model to be preserved")
	}
}

func TestSetDefaultPlugin_NewPlugin(t *testing.T) {
	lm := LMConfig{
		Plugins: map[string]PluginConfig{
			"claude-code": {Default: true},
		},
	}

	lm.SetDefaultPlugin("aider")

	if lm.Plugins["claude-code"].Default {
		t.Error("expected claude-code Default to be false")
	}
	if !lm.Plugins["aider"].Default {
		t.Error("expected aider Default to be true")
	}
}

func TestSetDefaultPlugin_NilPluginsMap(t *testing.T) {
	lm := LMConfig{}

	lm.SetDefaultPlugin("gemini")

	if lm.Plugins == nil {
		t.Fatal("expected Plugins map to be initialized")
	}
	if !lm.Plugins["gemini"].Default {
		t.Error("expected gemini Default to be true")
	}
}

func TestSetDefaultPlugin_OnlyOneDefault(t *testing.T) {
	lm := LMConfig{
		Plugins: map[string]PluginConfig{
			"claude-code": {Default: true},
			"gemini":      {Default: true},
			"aider":       {Default: false},
		},
	}

	lm.SetDefaultPlugin("aider")

	defaultCount := 0
	for _, cfg := range lm.Plugins {
		if cfg.Default {
			defaultCount++
		}
	}
	if defaultCount != 1 {
		t.Errorf("expected exactly 1 default, got %d", defaultCount)
	}
	if !lm.Plugins["aider"].Default {
		t.Error("expected aider to be the default")
	}
}

func TestResolveProfile_HooksInheritance(t *testing.T) {
	profiles := map[string]Profile{
		"base": {
			Hooks: HooksConfig{
				Unified: UnifiedHooks{
					PreTool: []Hook{
						{Command: "./base-hook.sh", Matcher: "Bash"},
					},
				},
			},
		},
		"child": {
			Parents: []string{"base"},
			Hooks: HooksConfig{
				Unified: UnifiedHooks{
					PostTool: []Hook{
						{Command: "./child-hook.sh", Matcher: "Edit"},
					},
				},
			},
		},
	}

	resolved, err := ResolveProfile(profiles, "child")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have inherited PreTool from base
	if len(resolved.Hooks.Unified.PreTool) != 1 {
		t.Errorf("expected 1 PreTool hook, got %d", len(resolved.Hooks.Unified.PreTool))
	}
	if resolved.Hooks.Unified.PreTool[0].Command != "./base-hook.sh" {
		t.Errorf("expected base hook command, got %s", resolved.Hooks.Unified.PreTool[0].Command)
	}

	// Should have own PostTool
	if len(resolved.Hooks.Unified.PostTool) != 1 {
		t.Errorf("expected 1 PostTool hook, got %d", len(resolved.Hooks.Unified.PostTool))
	}
}

func TestResolveProfile_HooksDeduplication(t *testing.T) {
	profiles := map[string]Profile{
		"base": {
			Hooks: HooksConfig{
				Unified: UnifiedHooks{
					PreTool: []Hook{
						{Command: "./shared-hook.sh", Matcher: "Bash"},
					},
				},
			},
		},
		"child": {
			Parents: []string{"base"},
			Hooks: HooksConfig{
				Unified: UnifiedHooks{
					PreTool: []Hook{
						{Command: "./shared-hook.sh", Matcher: "Bash"}, // Duplicate
						{Command: "./unique-hook.sh", Matcher: "Edit"},
					},
				},
			},
		},
	}

	resolved, err := ResolveProfile(profiles, "child")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have deduplicated PreTool hooks (2 unique, not 3)
	if len(resolved.Hooks.Unified.PreTool) != 2 {
		t.Errorf("expected 2 PreTool hooks after dedup, got %d", len(resolved.Hooks.Unified.PreTool))
	}
}

func TestResolveProfile_BackendHooksInheritance(t *testing.T) {
	profiles := map[string]Profile{
		"base": {
			Hooks: HooksConfig{
				Plugins: map[string]BackendHooks{
					"claude-code": {
						"PreToolUse": []Hook{
							{Command: "./base-claude.sh"},
						},
					},
				},
			},
		},
		"child": {
			Parents: []string{"base"},
			Hooks: HooksConfig{
				Plugins: map[string]BackendHooks{
					"claude-code": {
						"PostToolUse": []Hook{
							{Command: "./child-claude.sh"},
						},
					},
				},
			},
		},
	}

	resolved, err := ResolveProfile(profiles, "child")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have inherited claude-code PreToolUse from base
	if len(resolved.Hooks.Plugins["claude-code"]["PreToolUse"]) != 1 {
		t.Errorf("expected 1 PreToolUse hook, got %d", len(resolved.Hooks.Plugins["claude-code"]["PreToolUse"]))
	}

	// Should have own claude-code PostToolUse
	if len(resolved.Hooks.Plugins["claude-code"]["PostToolUse"]) != 1 {
		t.Errorf("expected 1 PostToolUse hook, got %d", len(resolved.Hooks.Plugins["claude-code"]["PostToolUse"]))
	}
}
