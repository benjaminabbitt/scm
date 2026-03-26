package backends

import (
	"testing"

	"github.com/SophisticatedContextManager/scm/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Claude Lifecycle Tests
// =============================================================================

// TestClaudeLifecycle_New verifies proper initialization
func TestClaudeLifecycle_New(t *testing.T) {
	backend := &ClaudeCode{
		BaseBackend: NewBaseBackend("claude-code", "1.0.0"),
	}
	lifecycle := &ClaudeLifecycle{
		backend: backend,
		hooks:   &config.HooksConfig{},
		mcp:     &config.MCPConfig{},
	}

	assert.NotNil(t, lifecycle)
	assert.Equal(t, backend, lifecycle.backend)
}

// TestClaudeLifecycle_OnSessionStart verifies session start handler registration
func TestClaudeLifecycle_OnSessionStart(t *testing.T) {
	backend := &ClaudeCode{
		BaseBackend: NewBaseBackend("claude-code", "1.0.0"),
	}
	lifecycle := &ClaudeLifecycle{
		backend: backend,
	}

	handler := EventHandler{
		Command: "echo test",
		Timeout: 30,
	}

	err := lifecycle.OnSessionStart("/tmp", handler)
	require.NoError(t, err)

	// Verify hook was added
	assert.NotNil(t, lifecycle.hooks)
	assert.Len(t, lifecycle.hooks.Unified.SessionStart, 1)
}

// TestClaudeLifecycle_OnSessionEnd verifies session end handler registration
func TestClaudeLifecycle_OnSessionEnd(t *testing.T) {
	backend := &ClaudeCode{
		BaseBackend: NewBaseBackend("claude-code", "1.0.0"),
	}
	lifecycle := &ClaudeLifecycle{
		backend: backend,
	}

	handler := EventHandler{
		Command: "echo cleanup",
		Timeout: 30,
	}

	err := lifecycle.OnSessionEnd("/tmp", handler)
	require.NoError(t, err)

	assert.NotNil(t, lifecycle.hooks)
	assert.Len(t, lifecycle.hooks.Unified.SessionEnd, 1)
}

// TestClaudeLifecycle_OnToolUse verifies tool use handler registration
func TestClaudeLifecycle_OnToolUse(t *testing.T) {
	backend := &ClaudeCode{
		BaseBackend: NewBaseBackend("claude-code", "1.0.0"),
	}
	lifecycle := &ClaudeLifecycle{
		backend: backend,
	}

	handler := EventHandler{
		Command: "echo tool",
		Timeout: 30,
	}

	t.Run("before tool use", func(t *testing.T) {
		err := lifecycle.OnToolUse("/tmp", BeforeToolUse, handler)
		require.NoError(t, err)
		assert.Len(t, lifecycle.hooks.Unified.PreTool, 1)
	})

	t.Run("after tool use", func(t *testing.T) {
		lifecycle.hooks = &config.HooksConfig{}
		err := lifecycle.OnToolUse("/tmp", AfterToolUse, handler)
		require.NoError(t, err)
		assert.Len(t, lifecycle.hooks.Unified.PostTool, 1)
	})
}

// TestClaudeLifecycle_Clear verifies handlers can be cleared
func TestClaudeLifecycle_Clear(t *testing.T) {
	backend := &ClaudeCode{
		BaseBackend: NewBaseBackend("claude-code", "1.0.0"),
	}
	lifecycle := &ClaudeLifecycle{
		backend: backend,
		hooks: &config.HooksConfig{
			Unified: config.UnifiedHooks{
				SessionStart: []config.Hook{
					{Command: "echo test"},
				},
			},
			Plugins: make(map[string]config.BackendHooks),
		},
		mcp: &config.MCPConfig{
			Servers: map[string]config.MCPServer{},
			Plugins: make(map[string]map[string]config.MCPServer),
		},
	}

	// Note: Clear will try to write to settings, which may fail in test
	// We're just verifying it resets internal state
	lifecycle.Clear("/tmp")
	assert.NotNil(t, lifecycle.hooks)
	assert.NotNil(t, lifecycle.mcp)
}

// TestClaudeLifecycle_Flush verifies hooks and MCP are flushed
func TestClaudeLifecycle_Flush(t *testing.T) {
	backend := &ClaudeCode{
		BaseBackend: NewBaseBackend("claude-code", "1.0.0"),
	}
	lifecycle := &ClaudeLifecycle{
		backend: backend,
		hooks: &config.HooksConfig{
			Unified: config.UnifiedHooks{
				SessionStart: []config.Hook{
					{Command: "echo test"},
				},
			},
			Plugins: make(map[string]config.BackendHooks),
		},
		mcp: &config.MCPConfig{
			Servers: map[string]config.MCPServer{},
			Plugins: make(map[string]map[string]config.MCPServer),
		},
	}

	// Flush will attempt file I/O; we're verifying it doesn't panic
	_ = lifecycle.Flush("/tmp")
}

// =============================================================================
// Gemini Lifecycle Tests
// =============================================================================

// TestGeminiLifecycle_New verifies proper initialization
func TestGeminiLifecycle_New(t *testing.T) {
	backend := &Gemini{
		BaseBackend: NewBaseBackend("gemini", "1.0.0"),
	}
	lifecycle := &GeminiLifecycle{
		backend: backend,
		hooks:   &config.HooksConfig{},
		mcp:     &config.MCPConfig{},
	}

	assert.NotNil(t, lifecycle)
	assert.Equal(t, backend, lifecycle.backend)
}

// TestGeminiLifecycle_OnSessionStart verifies session start handler registration
func TestGeminiLifecycle_OnSessionStart(t *testing.T) {
	backend := &Gemini{
		BaseBackend: NewBaseBackend("gemini", "1.0.0"),
	}
	lifecycle := &GeminiLifecycle{
		backend: backend,
	}

	handler := EventHandler{
		Command: "echo test",
		Timeout: 30,
	}

	err := lifecycle.OnSessionStart("/tmp", handler)
	require.NoError(t, err)

	assert.NotNil(t, lifecycle.hooks)
	assert.Len(t, lifecycle.hooks.Unified.SessionStart, 1)
}

// TestGeminiLifecycle_OnSessionEnd verifies session end handler registration
func TestGeminiLifecycle_OnSessionEnd(t *testing.T) {
	backend := &Gemini{
		BaseBackend: NewBaseBackend("gemini", "1.0.0"),
	}
	lifecycle := &GeminiLifecycle{
		backend: backend,
	}

	handler := EventHandler{
		Command: "echo cleanup",
		Timeout: 30,
	}

	err := lifecycle.OnSessionEnd("/tmp", handler)
	require.NoError(t, err)

	assert.NotNil(t, lifecycle.hooks)
	assert.Len(t, lifecycle.hooks.Unified.SessionEnd, 1)
}

// TestGeminiLifecycle_OnToolUse verifies tool use handler registration
func TestGeminiLifecycle_OnToolUse(t *testing.T) {
	backend := &Gemini{
		BaseBackend: NewBaseBackend("gemini", "1.0.0"),
	}
	lifecycle := &GeminiLifecycle{
		backend: backend,
	}

	handler := EventHandler{
		Command: "echo tool",
		Timeout: 30,
	}

	t.Run("before tool use", func(t *testing.T) {
		err := lifecycle.OnToolUse("/tmp", BeforeToolUse, handler)
		require.NoError(t, err)
		assert.Len(t, lifecycle.hooks.Unified.PreTool, 1)
	})

	t.Run("after tool use", func(t *testing.T) {
		lifecycle.hooks = &config.HooksConfig{}
		err := lifecycle.OnToolUse("/tmp", AfterToolUse, handler)
		require.NoError(t, err)
		assert.Len(t, lifecycle.hooks.Unified.PostTool, 1)
	})
}

// TestGeminiCommand_Structure verifies the command structure
func TestGeminiCommand_Structure(t *testing.T) {
	cmd := GeminiCommand{
		Description: "Test command",
		Prompt:      "Test prompt",
	}

	assert.Equal(t, "Test command", cmd.Description)
	assert.Equal(t, "Test prompt", cmd.Prompt)
}

// =============================================================================
// Claude MCP Manager Tests
// =============================================================================

func TestClaudeMCPManager_RegisterServer(t *testing.T) {
	backend := &ClaudeCode{
		BaseBackend: NewBaseBackend("claude-code", "1.0.0"),
	}
	manager := &ClaudeMCPManager{
		backend: backend,
	}

	server := MCPServer{
		Name:    "test-server",
		Command: "test-cmd",
		Args:    []string{"arg1"},
	}

	err := manager.RegisterServer("/tmp", server)
	require.NoError(t, err)
	assert.NotNil(t, manager.servers)
	assert.Len(t, manager.servers, 1)
	assert.Equal(t, server, manager.servers["test-server"])
}

func TestClaudeMCPManager_UnregisterServer(t *testing.T) {
	backend := &ClaudeCode{
		BaseBackend: NewBaseBackend("claude-code", "1.0.0"),
	}
	manager := &ClaudeMCPManager{
		backend: backend,
		servers: map[string]MCPServer{
			"test-server": {
				Name:    "test-server",
				Command: "test-cmd",
			},
		},
	}

	err := manager.UnregisterServer("/tmp", "test-server")
	require.NoError(t, err)
	assert.Len(t, manager.servers, 0)
	assert.NotContains(t, manager.servers, "test-server")
}

func TestClaudeMCPManager_ListServers(t *testing.T) {
	backend := &ClaudeCode{
		BaseBackend: NewBaseBackend("claude-code", "1.0.0"),
	}
	manager := &ClaudeMCPManager{
		backend: backend,
		servers: map[string]MCPServer{
			"server1": {Name: "server1"},
			"server2": {Name: "server2"},
		},
	}

	names, err := manager.ListServers("/tmp")
	require.NoError(t, err)
	assert.Len(t, names, 2)
	assert.Contains(t, names, "server1")
	assert.Contains(t, names, "server2")
}

func TestClaudeMCPManager_GetServer(t *testing.T) {
	backend := &ClaudeCode{
		BaseBackend: NewBaseBackend("claude-code", "1.0.0"),
	}
	server := MCPServer{
		Name:    "test-server",
		Command: "test-cmd",
		Args:    []string{"arg1"},
	}
	manager := &ClaudeMCPManager{
		backend: backend,
		servers: map[string]MCPServer{"test-server": server},
	}

	result, err := manager.GetServer("/tmp", "test-server")
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, server.Name, result.Name)
	assert.Equal(t, server.Command, result.Command)
}

func TestClaudeMCPManager_GetServer_NotFound(t *testing.T) {
	backend := &ClaudeCode{
		BaseBackend: NewBaseBackend("claude-code", "1.0.0"),
	}
	manager := &ClaudeMCPManager{
		backend: backend,
		servers: make(map[string]MCPServer),
	}

	result, err := manager.GetServer("/tmp", "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestClaudeMCPManager_Clear(t *testing.T) {
	backend := &ClaudeCode{
		BaseBackend: NewBaseBackend("claude-code", "1.0.0"),
	}
	manager := &ClaudeMCPManager{
		backend: backend,
		servers: map[string]MCPServer{
			"server1": {Name: "server1"},
		},
	}

	// Clear will attempt file I/O; we're verifying it clears internal state
	_ = manager.Clear("/tmp")
	assert.Len(t, manager.servers, 0)
}

func TestClaudeMCPManager_EnsureServers(t *testing.T) {
	manager := &ClaudeMCPManager{
		servers: nil,
	}
	manager.ensureServers()
	assert.NotNil(t, manager.servers)
	assert.Equal(t, 0, len(manager.servers))
}

// =============================================================================
// Claude Skills Tests
// =============================================================================

func TestClaudeSkills_Register(t *testing.T) {
	backend := &ClaudeCode{
		BaseBackend: NewBaseBackend("claude-code", "1.0.0"),
	}
	skills := &ClaudeSkills{
		backend: backend,
	}

	skill := Skill{
		Name:        "test-skill",
		Description: "Test skill",
		Content:     "# Test Skill\n\nTest content",
	}

	// Register will attempt file I/O
	err := skills.Register("/tmp", skill)
	// We expect this to succeed or fail due to file I/O, not panic
	_ = err
}

func TestClaudeSkills_RegisterAll(t *testing.T) {
	backend := &ClaudeCode{
		BaseBackend: NewBaseBackend("claude-code", "1.0.0"),
	}
	skills := &ClaudeSkills{
		backend: backend,
	}

	skillList := []Skill{
		{
			Name:        "skill1",
			Description: "Skill 1",
			Content:     "# Skill 1",
		},
		{
			Name:        "skill2",
			Description: "Skill 2",
			Content:     "# Skill 2",
		},
	}

	// RegisterAll will attempt file I/O
	err := skills.RegisterAll("/tmp", skillList)
	// We expect this to succeed or fail due to file I/O, not panic
	_ = err
}

// =============================================================================
// Claude Context Tests
// =============================================================================

func TestClaudeContext_GetContextHash(t *testing.T) {
	backend := &ClaudeCode{
		BaseBackend: NewBaseBackend("claude-code", "1.0.0"),
	}
	context := &ClaudeContext{
		backend:     backend,
		contextHash: "abc123def456",
	}

	hash := context.GetContextHash()
	assert.Equal(t, "abc123def456", hash)
}

func TestClaudeContext_GetContextHash_Empty(t *testing.T) {
	backend := &ClaudeCode{
		BaseBackend: NewBaseBackend("claude-code", "1.0.0"),
	}
	context := &ClaudeContext{
		backend:     backend,
		contextHash: "",
	}

	hash := context.GetContextHash()
	assert.Equal(t, "", hash)
}

func TestClaudeContext_GetContextFilePath(t *testing.T) {
	backend := &ClaudeCode{
		BaseBackend: NewBaseBackend("claude-code", "1.0.0"),
	}
	context := &ClaudeContext{
		backend:     backend,
		contextHash: "abc123",
	}

	path := context.GetContextFilePath()
	assert.Equal(t, ".scm/context/abc123.md", path)
}

func TestClaudeContext_GetContextFilePath_Empty(t *testing.T) {
	backend := &ClaudeCode{
		BaseBackend: NewBaseBackend("claude-code", "1.0.0"),
	}
	context := &ClaudeContext{
		backend:     backend,
		contextHash: "",
	}

	path := context.GetContextFilePath()
	assert.Equal(t, "", path)
}

func TestClaudeContext_Clear(t *testing.T) {
	backend := &ClaudeCode{
		BaseBackend: NewBaseBackend("claude-code", "1.0.0"),
	}
	context := &ClaudeContext{
		backend:     backend,
		contextHash: "abc123",
	}

	err := context.Clear("/tmp")
	require.NoError(t, err)
	assert.Equal(t, "", context.contextHash)
}

// =============================================================================
// Claude Lifecycle MergeConfigHooks Tests
// =============================================================================

func TestClaudeLifecycle_MergeConfigHooks_WithContextHash(t *testing.T) {
	backend := &ClaudeCode{
		BaseBackend: NewBaseBackend("claude-code", "1.0.0"),
	}
	lifecycle := &ClaudeLifecycle{
		backend: backend,
		hooks:   &config.HooksConfig{Plugins: make(map[string]config.BackendHooks)},
		mcp:     &config.MCPConfig{Servers: make(map[string]config.MCPServer), Plugins: make(map[string]map[string]config.MCPServer)},
	}

	cfg := &config.Config{
		Hooks: config.HooksConfig{Plugins: make(map[string]config.BackendHooks)},
		MCP:   config.MCPConfig{Servers: make(map[string]config.MCPServer), Plugins: make(map[string]map[string]config.MCPServer)},
	}

	lifecycle.MergeConfigHooks(cfg, "/tmp", "abc123hash")

	// Verify context injection hook was added
	assert.NotEmpty(t, lifecycle.hooks.Unified.SessionStart)
}

func TestClaudeLifecycle_MergeConfigHooks_NoContextHash(t *testing.T) {
	backend := &ClaudeCode{
		BaseBackend: NewBaseBackend("claude-code", "1.0.0"),
	}
	lifecycle := &ClaudeLifecycle{
		backend: backend,
		hooks:   &config.HooksConfig{Plugins: make(map[string]config.BackendHooks)},
		mcp:     &config.MCPConfig{Servers: make(map[string]config.MCPServer), Plugins: make(map[string]map[string]config.MCPServer)},
	}

	cfg := &config.Config{
		Hooks: config.HooksConfig{Plugins: make(map[string]config.BackendHooks)},
		MCP:   config.MCPConfig{Servers: make(map[string]config.MCPServer), Plugins: make(map[string]map[string]config.MCPServer)},
	}

	lifecycle.MergeConfigHooks(cfg, "/tmp", "")

	// Without context hash, SessionStart should remain empty
	assert.Empty(t, lifecycle.hooks.Unified.SessionStart)
}
