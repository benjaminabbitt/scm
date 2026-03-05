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

func TestResolveProfile_MCPInheritance(t *testing.T) {
	profiles := map[string]Profile{
		"base": {
			MCP: MCPConfig{
				Servers: map[string]MCPServer{
					"base-server": {
						Command: "base-server-cmd",
						Args:    []string{"--base"},
					},
				},
			},
		},
		"child": {
			Parents: []string{"base"},
			MCP: MCPConfig{
				Servers: map[string]MCPServer{
					"child-server": {
						Command: "child-server-cmd",
					},
				},
			},
		},
	}

	resolved, err := ResolveProfile(profiles, "child")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have inherited base server
	if _, ok := resolved.MCP.Servers["base-server"]; !ok {
		t.Error("expected base-server to be inherited")
	}
	if resolved.MCP.Servers["base-server"].Command != "base-server-cmd" {
		t.Errorf("expected base-server command, got %s", resolved.MCP.Servers["base-server"].Command)
	}

	// Should have own child server
	if _, ok := resolved.MCP.Servers["child-server"]; !ok {
		t.Error("expected child-server to be present")
	}
}

func TestResolveProfile_MCPOverride(t *testing.T) {
	profiles := map[string]Profile{
		"base": {
			MCP: MCPConfig{
				Servers: map[string]MCPServer{
					"shared-server": {
						Command: "base-cmd",
						Args:    []string{"--base"},
					},
				},
			},
		},
		"child": {
			Parents: []string{"base"},
			MCP: MCPConfig{
				Servers: map[string]MCPServer{
					"shared-server": {
						Command: "child-cmd",
						Args:    []string{"--child"},
					},
				},
			},
		},
	}

	resolved, err := ResolveProfile(profiles, "child")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Child should override base for same server name
	if resolved.MCP.Servers["shared-server"].Command != "child-cmd" {
		t.Errorf("expected child to override base server, got %s", resolved.MCP.Servers["shared-server"].Command)
	}
	if len(resolved.MCP.Servers["shared-server"].Args) != 1 || resolved.MCP.Servers["shared-server"].Args[0] != "--child" {
		t.Errorf("expected child args, got %v", resolved.MCP.Servers["shared-server"].Args)
	}
}

func TestResolveProfile_MCPAutoRegisterOverride(t *testing.T) {
	falseVal := false
	trueVal := true

	profiles := map[string]Profile{
		"base": {
			MCP: MCPConfig{
				AutoRegisterSCM: &trueVal,
			},
		},
		"child": {
			Parents: []string{"base"},
			MCP: MCPConfig{
				AutoRegisterSCM: &falseVal,
			},
		},
	}

	resolved, err := ResolveProfile(profiles, "child")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Child should override base's auto_register_scm
	if resolved.MCP.AutoRegisterSCM == nil {
		t.Fatal("expected AutoRegisterSCM to be set")
	}
	if *resolved.MCP.AutoRegisterSCM != false {
		t.Error("expected child to override AutoRegisterSCM to false")
	}
}

func TestResolveProfile_MCPBackendInheritance(t *testing.T) {
	profiles := map[string]Profile{
		"base": {
			MCP: MCPConfig{
				Plugins: map[string]map[string]MCPServer{
					"claude-code": {
						"base-claude-server": {
							Command: "base-claude-cmd",
						},
					},
				},
			},
		},
		"child": {
			Parents: []string{"base"},
			MCP: MCPConfig{
				Plugins: map[string]map[string]MCPServer{
					"claude-code": {
						"child-claude-server": {
							Command: "child-claude-cmd",
						},
					},
					"gemini": {
						"gemini-server": {
							Command: "gemini-cmd",
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

	// Should have inherited claude-code base server
	if _, ok := resolved.MCP.Plugins["claude-code"]["base-claude-server"]; !ok {
		t.Error("expected base-claude-server to be inherited")
	}

	// Should have own claude-code child server
	if _, ok := resolved.MCP.Plugins["claude-code"]["child-claude-server"]; !ok {
		t.Error("expected child-claude-server to be present")
	}

	// Should have gemini server
	if _, ok := resolved.MCP.Plugins["gemini"]["gemini-server"]; !ok {
		t.Error("expected gemini-server to be present")
	}
}
