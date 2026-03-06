package remote

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Note: TestItemType_DirName and TestItemType_Plural are in browse_test.go

func TestItemType_DirName_CustomType(t *testing.T) {
	// Test the default case for custom item types
	assert.Equal(t, "customs", ItemType("custom").DirName())
}

func TestRemoteMCPServer_SecurityWarning(t *testing.T) {
	server := RemoteMCPServer{
		Command: "test-command",
		Args:    []string{"arg1"},
	}

	warning := server.SecurityWarning()
	assert.Equal(t, "MCP SERVER INSTALLATION", warning.Title)
	assert.Contains(t, warning.Context, "execute commands")
	assert.GreaterOrEqual(t, len(warning.Risks), 3)
}

func TestRemoteMCPServer_Note(t *testing.T) {
	server := RemoteMCPServer{
		NoteField: "This is a test note",
	}
	assert.Equal(t, "This is a test note", server.Note())
}

func TestRemoteContext_SecurityWarning(t *testing.T) {
	ctx := RemoteContext{}

	warning := ctx.SecurityWarning()
	assert.Equal(t, "PROMPT INJECTION RISK", warning.Title)
	assert.Contains(t, warning.Context, "influence AI behavior")
	assert.GreaterOrEqual(t, len(warning.Risks), 3)
}

func TestRemoteContext_Note(t *testing.T) {
	ctx := RemoteContext{NoteField: "Context note"}
	assert.Equal(t, "Context note", ctx.Note())
}

func TestRemoteBundle_SecurityWarning(t *testing.T) {
	t.Run("bundle without MCP", func(t *testing.T) {
		bundle := RemoteBundle{
			Version:   "1.0",
			Fragments: map[string]RemoteBundleItem{"test": {Content: "content"}},
		}

		warning := bundle.SecurityWarning()
		assert.Equal(t, "BUNDLE INSTALLATION", warning.Title)
		assert.Contains(t, warning.Context, "AI context")
		assert.GreaterOrEqual(t, len(warning.Risks), 3)
	})

	t.Run("bundle with MCP", func(t *testing.T) {
		bundle := RemoteBundle{
			Version: "1.0",
			MCP:     &RemoteMCPServer{Command: "test-cmd"},
		}

		warning := bundle.SecurityWarning()
		assert.Equal(t, "BUNDLE INSTALLATION (WITH MCP SERVER)", warning.Title)
		assert.Contains(t, warning.Context, "executable code")
		assert.GreaterOrEqual(t, len(warning.Risks), 5) // Has more risks when MCP included
	})

	t.Run("bundle with empty MCP", func(t *testing.T) {
		bundle := RemoteBundle{
			Version: "1.0",
			MCP:     &RemoteMCPServer{}, // Empty command
		}

		warning := bundle.SecurityWarning()
		assert.Equal(t, "BUNDLE INSTALLATION", warning.Title) // No MCP warning
	})
}

func TestRemoteBundle_Note(t *testing.T) {
	bundle := RemoteBundle{NoteField: "Bundle note"}
	assert.Equal(t, "Bundle note", bundle.Note())
}

func TestRemoteBundle_HasMCP(t *testing.T) {
	tests := []struct {
		name     string
		bundle   RemoteBundle
		expected bool
	}{
		{"no MCP", RemoteBundle{}, false},
		{"nil MCP", RemoteBundle{MCP: nil}, false},
		{"empty MCP command", RemoteBundle{MCP: &RemoteMCPServer{}}, false},
		{"valid MCP", RemoteBundle{MCP: &RemoteMCPServer{Command: "cmd"}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.bundle.HasMCP())
		})
	}
}

func TestParseSecureContent_Bundle(t *testing.T) {
	yaml := `
version: "1.0"
description: Test bundle
notes: Test note
mcp:
  command: test-cmd
  args: [arg1]
fragments:
  test-frag:
    content: Fragment content
    tags: [test]
prompts:
  test-prompt:
    content: Prompt content
`
	content, err := ParseSecureContent(ItemTypeBundle, []byte(yaml))
	require.NoError(t, err)

	bundle, ok := content.(RemoteBundle)
	require.True(t, ok)
	assert.Equal(t, "1.0", bundle.Version)
	assert.Equal(t, "Test bundle", bundle.Description)
	assert.Equal(t, "Test note", bundle.Note())
	assert.True(t, bundle.HasMCP())
	assert.Equal(t, "test-cmd", bundle.MCP.Command)
	assert.Len(t, bundle.Fragments, 1)
	assert.Len(t, bundle.Prompts, 1)
}

func TestParseSecureContent_Profile(t *testing.T) {
	yaml := `
note: Profile note
`
	content, err := ParseSecureContent(ItemTypeProfile, []byte(yaml))
	require.NoError(t, err)

	ctx, ok := content.(RemoteContext)
	require.True(t, ok)
	assert.Equal(t, "Profile note", ctx.Note())
}

func TestParseSecureContent_DefaultType(t *testing.T) {
	yaml := `note: Context note`
	content, err := ParseSecureContent(ItemType("other"), []byte(yaml))
	require.NoError(t, err)

	ctx, ok := content.(RemoteContext)
	require.True(t, ok)
	assert.Equal(t, "Context note", ctx.Note())
}

func TestParseSecureContent_InvalidYAML(t *testing.T) {
	invalidYAML := `invalid: yaml: content: [[`
	_, err := ParseSecureContent(ItemTypeBundle, []byte(invalidYAML))
	require.Error(t, err)
}
