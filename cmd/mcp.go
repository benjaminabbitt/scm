package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/benjaminabbitt/scm/internal/bundles"
	"github.com/benjaminabbitt/scm/internal/collections"
	"github.com/benjaminabbitt/scm/internal/config"
	"github.com/benjaminabbitt/scm/internal/gitutil"
	"github.com/benjaminabbitt/scm/internal/lm/backends"
	"github.com/benjaminabbitt/scm/internal/profiles"
	"github.com/benjaminabbitt/scm/internal/remote"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run as MCP server over stdio",
	Long: `Run scm as an MCP (Model Context Protocol) server over stdio.

This allows AI agents to interact with scm functionality using standard MCP tool calls.

Available tools:
  Context:
  - list_fragments: List available context fragments
  - get_fragment: Get a fragment's content by name
  - create_fragment: Create a new context fragment
  - delete_fragment: Delete a local fragment
  - assemble_context: Assemble context from profile/fragments/tags

  Profiles:
  - list_profiles: List configured profiles
  - get_profile: Get a profile's configuration
  - create_profile: Create a new profile
  - update_profile: Update an existing profile
  - delete_profile: Delete a profile

  Prompts:
  - list_prompts: List saved prompts
  - get_prompt: Get a prompt's content by name

  Search:
  - search_content: Search across all content types (fragments, prompts, profiles, MCP servers)

  Hooks:
  - apply_hooks: Apply/reapply SCM hooks to backend configs

  MCP Servers:
  - list_mcp_servers: List configured MCP servers
  - add_mcp_server: Add an MCP server to config
  - remove_mcp_server: Remove an MCP server from config
  - set_mcp_auto_register: Enable/disable SCM MCP auto-registration

  Remotes:
  - list_remotes: List configured remote sources
  - add_remote: Register a new remote source
  - remove_remote: Remove a remote source
  - discover_remotes: Search GitHub/GitLab for SCM repositories
  - browse_remote: List items available in a remote
  - preview_remote: Preview content before pulling
  - confirm_pull: Install a previewed item

  Lockfile:
  - lock_dependencies: Generate lockfile from installed items
  - install_dependencies: Install items from lockfile
  - check_outdated: Check for newer versions

Example:
  scm mcp`,
	RunE: runMCPServer,
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

// MCP JSON-RPC types

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *mcpError   `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpToolInfo struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

type mcpToolResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func runMCPServer(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	server := &mcpServer{
		reader:       bufio.NewReader(os.Stdin),
		writer:       os.Stdout,
		pendingPulls: make(map[string]*pendingPull),
	}

	return server.run(ctx)
}

type mcpServer struct {
	reader       *bufio.Reader
	writer       io.Writer
	cfg          *config.Config
	pendingPulls map[string]*pendingPull // token -> pending pull info
	pullMu       sync.RWMutex
}

// pendingPull stores preview data awaiting confirmation.
type pendingPull struct {
	Reference string          `json:"reference"` // remote/path@SHA
	ItemType  remote.ItemType `json:"item_type"`
	Content   []byte          `json:"content"`
	SHA       string          `json:"sha"`
	RemoteURL string          `json:"remote_url"`
}

// bundleLoader returns a bundle loader configured for the current config.
func (s *mcpServer) bundleLoader() *bundles.Loader {
	return bundles.NewLoader(s.cfg.GetBundleDirs(), s.cfg.Defaults.ShouldUseDistilled()).
		WithLegacyDirs(s.cfg.GetFragmentDirs()).
		WithLegacyPromptDirs(s.cfg.GetPromptDirs())
}

// profileLoader returns a profiles.Loader for directory-based profiles.
func (s *mcpServer) profileLoader() *profiles.Loader {
	return s.cfg.GetProfileLoader()
}

// loadProfile loads a profile by name, checking both config map and directory.
// Config map profiles take precedence over directory-based profiles.
func (s *mcpServer) loadProfile(name string) (*config.Profile, error) {
	// First check config map
	if profile, ok := s.cfg.Profiles[name]; ok {
		return &profile, nil
	}

	// Fall back to directory-based profile
	loader := s.profileLoader()
	dirProfile, err := loader.Load(name)
	if err != nil {
		return nil, fmt.Errorf("unknown profile: %s", name)
	}

	// Convert profiles.Profile to config.Profile
	return &config.Profile{
		Description: dirProfile.Description,
		Parents:     dirProfile.Parents,
		Tags:        dirProfile.Tags,
		Fragments:   dirProfile.Bundles, // Bundles field contains fragment references
		Variables:   dirProfile.Variables,
		Generators:  dirProfile.Generators,
	}, nil
}

// resolveProfile resolves a profile with inheritance, checking both sources.
func (s *mcpServer) resolveProfile(name string) (*config.Profile, error) {
	// First try config-based resolution
	profile, err := config.ResolveProfile(s.cfg.Profiles, name)
	if err == nil {
		return profile, nil
	}

	// Fall back to directory-based resolution
	loader := s.profileLoader()
	resolved, err := loader.ResolveProfile(name, nil)
	if err != nil {
		return nil, fmt.Errorf("unknown profile: %s", name)
	}

	// Convert to config.Profile
	return &config.Profile{
		Tags:       resolved.Tags,
		Fragments:  resolved.Bundles,
		Variables:  resolved.Variables,
		Generators: resolved.Generators,
	}, nil
}

func (s *mcpServer) run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		line, err := s.reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		var req mcpRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.sendError(nil, -32700, "Parse error")
			continue
		}

		resp := s.handleRequest(ctx, &req)
		if resp != nil {
			s.sendResponse(resp)
		}
	}
}

func (s *mcpServer) sendResponse(resp *mcpResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		// Marshal error - send a minimal error response
		fmt.Fprintf(os.Stderr, "MCP: failed to marshal response: %v\n", err)
		fmt.Fprintln(s.writer, `{"jsonrpc":"2.0","error":{"code":-32603,"message":"internal marshal error"}}`)
		return
	}
	fmt.Fprintln(s.writer, string(data))
}

func (s *mcpServer) sendError(id interface{}, code int, message string) {
	resp := &mcpResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &mcpError{Code: code, Message: message},
	}
	s.sendResponse(resp)
}

func (s *mcpServer) handleRequest(ctx context.Context, req *mcpRequest) *mcpResponse {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "notifications/initialized":
		return nil
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(ctx, req)
	default:
		return &mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &mcpError{Code: -32601, Message: "Method not found: " + req.Method},
		}
	}
}

func (s *mcpServer) handleInitialize(req *mcpRequest) *mcpResponse {
	cfg, err := config.Load()
	if err != nil {
		return &mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &mcpError{Code: -32603, Message: "Failed to load config: " + err.Error()},
		}
	}
	s.cfg = cfg

	return &mcpResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"serverInfo": map[string]interface{}{
				"name":    "scm",
				"version": Version,
			},
		},
	}
}

func (s *mcpServer) handleToolsList(req *mcpRequest) *mcpResponse {
	tools := append(s.getLocalTools(), s.getRemoteTools()...)

	return &mcpResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"tools": tools,
		},
	}
}

func (s *mcpServer) getRemoteTools() []mcpToolInfo {
	return []mcpToolInfo{
		{
			Name:        "list_remotes",
			Description: "List configured remote sources for fragments and prompts",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "discover_remotes",
			Description: "Search GitHub/GitLab for SCM repositories containing fragments and prompts",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Optional search term to filter repositories",
					},
					"source": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"github", "gitlab", "all"},
						"description": "Which forge to search (default: all)",
					},
					"min_stars": map[string]interface{}{
						"type":        "integer",
						"description": "Minimum star count filter (default: 0)",
					},
				},
			},
		},
		{
			Name:        "browse_remote",
			Description: "List items (fragments, prompts, profiles) available in a remote repository",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"remote"},
				"properties": map[string]interface{}{
					"remote": map[string]interface{}{
						"type":        "string",
						"description": "Remote name (from list_remotes)",
					},
					"item_type": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"fragment", "prompt", "profile"},
						"description": "Type of items to list (default: all)",
					},
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Subdirectory path to browse (optional)",
					},
				},
			},
		},
		{
			Name:        "preview_remote",
			Description: "Preview content of a remote item before pulling. Returns a pull_token for confirm_pull.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"reference", "item_type"},
				"properties": map[string]interface{}{
					"reference": map[string]interface{}{
						"type":        "string",
						"description": "Remote reference (e.g., 'github/general/tdd' or 'github/security@v1.0.0')",
					},
					"item_type": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"fragment", "prompt", "profile"},
						"description": "Type of item to preview",
					},
				},
			},
		},
		{
			Name:        "confirm_pull",
			Description: "Install a previously previewed item using the pull_token from preview_remote",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"pull_token"},
				"properties": map[string]interface{}{
					"pull_token": map[string]interface{}{
						"type":        "string",
						"description": "Token from preview_remote response",
					},
				},
			},
		},
	}
}

func (s *mcpServer) getLocalTools() []mcpToolInfo {
	return []mcpToolInfo{
		{
			Name:        "list_fragments",
			Description: "List available local context fragments with their tags and source locations",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Text search on name (optional)",
					},
					"tags": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Filter by tags (optional)",
					},
					"sort_by": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"name", "source"},
						"description": "Sort field (default: name)",
					},
					"sort_order": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"asc", "desc"},
						"description": "Sort order (default: asc)",
					},
				},
			},
		},
		{
			Name:        "get_fragment",
			Description: "Get a local fragment's content by name",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"name"},
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Fragment name (without extension)",
					},
				},
			},
		},
		{
			Name:        "list_profiles",
			Description: "List all configured profiles with their descriptions",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Text search on name or description (optional)",
					},
					"sort_by": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"name", "default"},
						"description": "Sort field (default: name)",
					},
					"sort_order": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"asc", "desc"},
						"description": "Sort order (default: asc)",
					},
				},
			},
		},
		{
			Name:        "get_profile",
			Description: "Get a profile's configuration including fragments, tags, and variables",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"name"},
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Profile name",
					},
				},
			},
		},
		{
			Name:        "assemble_context",
			Description: "Assemble context from a profile, fragments, and/or tags. Returns the combined context that would be sent to an AI.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"profile": map[string]interface{}{
						"type":        "string",
						"description": "Profile name to use (optional)",
					},
					"fragments": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Additional fragment names to include (optional)",
					},
					"tags": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Include all fragments with these tags (optional)",
					},
				},
			},
		},
		{
			Name:        "list_prompts",
			Description: "List saved prompts",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Text search on name (optional)",
					},
					"sort_by": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"name"},
						"description": "Sort field (default: name)",
					},
					"sort_order": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"asc", "desc"},
						"description": "Sort order (default: asc)",
					},
				},
			},
		},
		{
			Name:        "get_prompt",
			Description: "Get a saved prompt's content by name",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"name"},
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Prompt name (without extension)",
					},
				},
			},
		},
		{
			Name:        "search_content",
			Description: "Search across all SCM content types (fragments, prompts, profiles, MCP servers)",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"query"},
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Search text (matches name, description, tags)",
					},
					"types": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string", "enum": []string{"fragment", "prompt", "profile", "mcp_server"}},
						"description": "Content types to search (default: all)",
					},
					"tags": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Filter by tags (fragments only)",
					},
					"sort_by": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"name", "type", "relevance"},
						"description": "Sort field (default: relevance)",
					},
					"sort_order": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"asc", "desc"},
						"description": "Sort order (default: asc)",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum results to return (default: 50)",
					},
				},
			},
		},
		// Profile management
		{
			Name:        "create_profile",
			Description: "Create a new profile with fragments, tags, and/or parent profiles",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"name"},
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Profile name",
					},
					"description": map[string]interface{}{
						"type":        "string",
						"description": "Profile description (optional)",
					},
					"parents": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Parent profiles to inherit from (optional)",
					},
					"fragments": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Fragment names to include (optional)",
					},
					"tags": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Tags to include fragments by (optional)",
					},
					"generators": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Generator names to run (optional)",
					},
					"default": map[string]interface{}{
						"type":        "boolean",
						"description": "Set as default profile (optional)",
					},
				},
			},
		},
		{
			Name:        "update_profile",
			Description: "Update an existing profile by adding/removing fragments, tags, or parents",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"name"},
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Profile name to update",
					},
					"description": map[string]interface{}{
						"type":        "string",
						"description": "New description (optional)",
					},
					"add_parents": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Parent profiles to add (optional)",
					},
					"remove_parents": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Parent profiles to remove (optional)",
					},
					"add_fragments": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Fragments to add (optional)",
					},
					"remove_fragments": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Fragments to remove (optional)",
					},
					"add_tags": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Tags to add (optional)",
					},
					"remove_tags": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Tags to remove (optional)",
					},
					"add_generators": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Generators to add (optional)",
					},
					"remove_generators": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Generators to remove (optional)",
					},
					"default": map[string]interface{}{
						"type":        "boolean",
						"description": "Set as default profile (optional)",
					},
				},
			},
		},
		{
			Name:        "delete_profile",
			Description: "Delete a profile",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"name"},
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Profile name to delete",
					},
				},
			},
		},
		// Fragment management
		{
			Name:        "create_fragment",
			Description: "Create a new context fragment",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"name", "content"},
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Fragment name (without extension)",
					},
					"content": map[string]interface{}{
						"type":        "string",
						"description": "Fragment content (markdown)",
					},
					"tags": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Tags for the fragment (optional)",
					},
					"version": map[string]interface{}{
						"type":        "string",
						"description": "Version string (optional, default: 1.0)",
					},
				},
			},
		},
		{
			Name:        "delete_fragment",
			Description: "Delete a local context fragment",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"name"},
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Fragment name to delete",
					},
				},
			},
		},
		// Hooks management
		{
			Name:        "apply_hooks",
			Description: "Apply/reapply SCM hooks to backend configuration files (.claude/settings.json, .gemini/settings.json)",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"backend": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"claude-code", "gemini", "all"},
						"description": "Which backend to apply hooks for (default: all)",
					},
					"regenerate_context": map[string]interface{}{
						"type":        "boolean",
						"description": "Also regenerate the context file (default: true)",
					},
				},
			},
		},
		// Remote management
		{
			Name:        "add_remote",
			Description: "Register a new remote source for fragments and prompts",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"name", "url"},
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Short name for the remote (e.g., 'alice')",
					},
					"url": map[string]interface{}{
						"type":        "string",
						"description": "Repository URL (e.g., 'alice/scm' or 'https://github.com/alice/scm')",
					},
				},
			},
		},
		{
			Name:        "remove_remote",
			Description: "Remove a registered remote source",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"name"},
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Remote name to remove",
					},
				},
			},
		},
		// MCP server management
		{
			Name:        "list_mcp_servers",
			Description: "List configured MCP servers",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Text search on name or command (optional)",
					},
					"sort_by": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"name", "command"},
						"description": "Sort field (default: name)",
					},
					"sort_order": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"asc", "desc"},
						"description": "Sort order (default: asc)",
					},
				},
			},
		},
		{
			Name:        "add_mcp_server",
			Description: "Add an MCP server to the configuration",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"name", "command"},
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Server name (unique identifier)",
					},
					"command": map[string]interface{}{
						"type":        "string",
						"description": "Command to run the MCP server",
					},
					"args": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Command arguments (optional)",
					},
					"backend": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"unified", "claude-code", "gemini"},
						"description": "Backend to add server for (default: unified for all backends)",
					},
				},
			},
		},
		{
			Name:        "remove_mcp_server",
			Description: "Remove an MCP server from the configuration",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"name"},
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Server name to remove",
					},
					"backend": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"unified", "claude-code", "gemini"},
						"description": "Backend to remove server from (default: all)",
					},
				},
			},
		},
		{
			Name:        "set_mcp_auto_register",
			Description: "Enable or disable auto-registration of SCM's own MCP server",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"enabled"},
				"properties": map[string]interface{}{
					"enabled": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to auto-register SCM's MCP server",
					},
				},
			},
		},
		// Lockfile management
		{
			Name:        "lock_dependencies",
			Description: "Generate a lockfile from currently installed remote items for reproducible installations",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "install_dependencies",
			Description: "Install all items from the lockfile",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"force": map[string]interface{}{
						"type":        "boolean",
						"description": "Skip confirmation prompts (default: false)",
					},
				},
			},
		},
		{
			Name:        "check_outdated",
			Description: "Check if any locked items have newer versions available",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
	}
}

func (s *mcpServer) handleToolsCall(ctx context.Context, req *mcpRequest) *mcpResponse {
	if s.cfg == nil {
		return &mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &mcpError{Code: -32002, Message: "Server not initialized"},
		}
	}

	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &mcpError{Code: -32602, Message: "Invalid params"},
		}
	}

	var result interface{}
	var err error

	switch params.Name {
	// Local tools
	case "list_fragments":
		result, err = s.toolListFragments(params.Arguments)
	case "get_fragment":
		result, err = s.toolGetFragment(params.Arguments)
	case "list_profiles":
		result, err = s.toolListProfiles(params.Arguments)
	case "get_profile":
		result, err = s.toolGetProfile(params.Arguments)
	case "assemble_context":
		result, err = s.toolAssembleContext(params.Arguments)
	case "list_prompts":
		result, err = s.toolListPrompts(params.Arguments)
	case "get_prompt":
		result, err = s.toolGetPrompt(params.Arguments)
	case "search_content":
		result, err = s.toolSearchContent(params.Arguments)
	// Profile management
	case "create_profile":
		result, err = s.toolCreateProfile(params.Arguments)
	case "update_profile":
		result, err = s.toolUpdateProfile(params.Arguments)
	case "delete_profile":
		result, err = s.toolDeleteProfile(params.Arguments)
	// Fragment management
	case "create_fragment":
		result, err = s.toolCreateFragment(params.Arguments)
	case "delete_fragment":
		result, err = s.toolDeleteFragment(params.Arguments)
	// Hooks management
	case "apply_hooks":
		result, err = s.toolApplyHooks(ctx, params.Arguments)
	// Remote tools
	case "list_remotes":
		result, err = s.toolListRemotes(ctx, params.Arguments)
	case "discover_remotes":
		result, err = s.toolDiscoverRemotes(ctx, params.Arguments)
	case "browse_remote":
		result, err = s.toolBrowseRemote(ctx, params.Arguments)
	case "preview_remote":
		result, err = s.toolPreviewRemote(ctx, params.Arguments)
	case "confirm_pull":
		result, err = s.toolConfirmPull(ctx, params.Arguments)
	// Remote management
	case "add_remote":
		result, err = s.toolAddRemote(ctx, params.Arguments)
	case "remove_remote":
		result, err = s.toolRemoveRemote(ctx, params.Arguments)
	// MCP server management
	case "list_mcp_servers":
		result, err = s.toolListMCPServers(ctx, params.Arguments)
	case "add_mcp_server":
		result, err = s.toolAddMCPServer(ctx, params.Arguments)
	case "remove_mcp_server":
		result, err = s.toolRemoveMCPServer(ctx, params.Arguments)
	case "set_mcp_auto_register":
		result, err = s.toolSetMCPAutoRegister(ctx, params.Arguments)
	// Lockfile management
	case "lock_dependencies":
		result, err = s.toolLockDependencies(ctx, params.Arguments)
	case "install_dependencies":
		result, err = s.toolInstallDependencies(ctx, params.Arguments)
	case "check_outdated":
		result, err = s.toolCheckOutdated(ctx, params.Arguments)
	default:
		return &mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &mcpError{Code: -32602, Message: "Unknown tool: " + params.Name},
		}
	}

	if err != nil {
		return &mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: mcpToolResult{
				Content: []mcpContent{{Type: "text", Text: "Error: " + err.Error()}},
				IsError: true,
			},
		}
	}

	text, _ := json.MarshalIndent(result, "", "  ")
	return &mcpResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: string(text)}},
		},
	}
}

// ============================================================================
// Tool implementations
// ============================================================================

func (s *mcpServer) toolListFragments(args json.RawMessage) (interface{}, error) {
	var params struct {
		Query     string   `json:"query"`
		Tags      []string `json:"tags"`
		SortBy    string   `json:"sort_by"`
		SortOrder string   `json:"sort_order"`
	}
	_ = json.Unmarshal(args, &params)

	loader := s.bundleLoader()

	var infos []bundles.ContentInfo
	var err error

	if len(params.Tags) > 0 {
		infos, err = loader.ListByTags(params.Tags)
	} else {
		infos, err = loader.ListAllFragments()
	}
	if err != nil {
		return nil, err
	}

	// Filter by query if provided
	if params.Query != "" {
		query := strings.ToLower(params.Query)
		var filtered []bundles.ContentInfo
		for _, info := range infos {
			if strings.Contains(strings.ToLower(info.Name), query) ||
				containsTag(info.Tags, query) {
				filtered = append(filtered, info)
			}
		}
		infos = filtered
	}

	// Sort results
	sortContentInfos(infos, params.SortBy, params.SortOrder)

	type fragmentEntry struct {
		Name   string   `json:"name"`
		Tags   []string `json:"tags,omitempty"`
		Source string   `json:"source"`
	}

	var result []fragmentEntry
	for _, info := range infos {
		result = append(result, fragmentEntry{
			Name:   info.Name,
			Tags:   info.Tags,
			Source: info.Source,
		})
	}

	return map[string]interface{}{
		"fragments": result,
		"count":     len(result),
	}, nil
}

// containsTag checks if any tag contains the query string.
func containsTag(tags []string, query string) bool {
	for _, tag := range tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}
	return false
}

// sortContentInfos sorts content infos by the specified field and order.
func sortContentInfos(infos []bundles.ContentInfo, sortBy, sortOrder string) {
	if sortBy == "" {
		sortBy = "name"
	}
	reverse := sortOrder == "desc"

	switch sortBy {
	case "name":
		sortSlice(infos, func(i, j int) bool {
			cmp := strings.Compare(strings.ToLower(infos[i].Name), strings.ToLower(infos[j].Name))
			if reverse {
				return cmp > 0
			}
			return cmp < 0
		})
	case "source":
		sortSlice(infos, func(i, j int) bool {
			cmp := strings.Compare(infos[i].Source, infos[j].Source)
			if reverse {
				return cmp > 0
			}
			return cmp < 0
		})
	}
}

// sortSlice is a helper that sorts a slice using a comparison function.
func sortSlice[T any](s []T, less func(i, j int) bool) {
	for i := 0; i < len(s)-1; i++ {
		for j := i + 1; j < len(s); j++ {
			if less(j, i) {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}

func (s *mcpServer) toolGetFragment(args json.RawMessage) (interface{}, error) {
	var params struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	loader := s.bundleLoader()

	content, err := loader.GetFragment(params.Name)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"name":    content.Name,
		"tags":    content.Tags,
		"content": content.Content,
	}, nil
}

func (s *mcpServer) toolListProfiles(args json.RawMessage) (interface{}, error) {
	var params struct {
		Query     string `json:"query"`
		SortBy    string `json:"sort_by"`
		SortOrder string `json:"sort_order"`
	}
	_ = json.Unmarshal(args, &params)

	type profileEntry struct {
		Name        string   `json:"name"`
		Description string   `json:"description,omitempty"`
		Tags        []string `json:"tags,omitempty"`
		Default     bool     `json:"default,omitempty"`
		Source      string   `json:"source,omitempty"` // "config" or "directory"
	}

	var result []profileEntry
	seen := collections.NewSet[string]()
	query := strings.ToLower(params.Query)
	defaultProfile := s.cfg.Defaults.Profile

	// Add config-based profiles first (they take precedence)
	for name, profile := range s.cfg.Profiles {
		seen.Add(name)
		// Filter by query if provided
		if query != "" {
			if !strings.Contains(strings.ToLower(name), query) &&
				!strings.Contains(strings.ToLower(profile.Description), query) {
				continue
			}
		}
		result = append(result, profileEntry{
			Name:        name,
			Description: profile.Description,
			Tags:        profile.Tags,
			Default:     name == defaultProfile,
			Source:      "config",
		})
	}

	// Add directory-based profiles
	loader := s.profileLoader()
	dirProfiles, err := loader.List()
	if err == nil {
		for _, p := range dirProfiles {
			if seen.Has(p.Name) {
				continue // Config profiles take precedence
			}
			// Filter by query if provided
			if query != "" {
				if !strings.Contains(strings.ToLower(p.Name), query) &&
					!strings.Contains(strings.ToLower(p.Description), query) {
					continue
				}
			}
			result = append(result, profileEntry{
				Name:        p.Name,
				Description: p.Description,
				Tags:        p.Tags,
				Default:     p.Name == defaultProfile,
				Source:      "directory",
			})
		}
	}

	// Sort results
	sortBy := params.SortBy
	if sortBy == "" {
		sortBy = "name"
	}
	reverse := params.SortOrder == "desc"

	switch sortBy {
	case "name":
		sortSlice(result, func(i, j int) bool {
			cmp := strings.Compare(strings.ToLower(result[i].Name), strings.ToLower(result[j].Name))
			if reverse {
				return cmp > 0
			}
			return cmp < 0
		})
	case "default":
		sortSlice(result, func(i, j int) bool {
			if reverse {
				return !result[i].Default && result[j].Default
			}
			return result[i].Default && !result[j].Default
		})
	}

	return map[string]interface{}{
		"profiles": result,
		"count":    len(result),
		"defaults": s.cfg.GetDefaultProfiles(),
	}, nil
}

func (s *mcpServer) toolGetProfile(args json.RawMessage) (interface{}, error) {
	var params struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	profile, err := s.loadProfile(params.Name)
	if err != nil {
		return nil, fmt.Errorf("profile not found: %s", params.Name)
	}

	return map[string]interface{}{
		"name":        params.Name,
		"description": profile.Description,
		"parents":     profile.Parents,
		"tags":        profile.Tags,
		"fragments":   profile.Fragments,
		"variables":   profile.Variables,
		"generators":  profile.Generators,
	}, nil
}

func (s *mcpServer) toolAssembleContext(args json.RawMessage) (interface{}, error) {
	var params struct {
		Profile   string   `json:"profile"`
		Fragments []string `json:"fragments"`
		Tags      []string `json:"tags"`
	}
	// Unmarshal errors are non-fatal - use defaults for optional params
	_ = json.Unmarshal(args, &params)

	loader := s.bundleLoader()

	var allFragments []string
	profileVars := make(map[string]string)

	profileName := params.Profile
	var profileNames []string
	if profileName == "" && len(params.Fragments) == 0 && len(params.Tags) == 0 {
		profileNames = s.cfg.GetDefaultProfiles()
	} else if profileName != "" {
		profileNames = []string{profileName}
	}

	// Process all profiles
	for _, pName := range profileNames {
		// Resolve profile with inheritance (checks both config and directory)
		profile, err := s.resolveProfile(pName)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve profile %s: %w", pName, err)
		}

		// Collect variables from profile
		for k, v := range profile.Variables {
			profileVars[k] = v
		}

		if len(profile.Tags) > 0 {
			taggedInfos, err := loader.ListByTags(profile.Tags)
			if err != nil {
				return nil, fmt.Errorf("failed to list fragments by profile tags: %w", err)
			}
			for _, info := range taggedInfos {
				allFragments = append(allFragments, info.Name)
			}
		}

		allFragments = append(allFragments, profile.Fragments...)
	}

	allFragments = append(allFragments, params.Fragments...)

	if len(params.Tags) > 0 {
		taggedInfos, err := loader.ListByTags(params.Tags)
		if err != nil {
			return nil, fmt.Errorf("failed to list fragments by tags: %w", err)
		}
		for _, info := range taggedInfos {
			allFragments = append(allFragments, info.Name)
		}
	}

	seen := collections.NewSet[string]()
	var uniqueFragments []string
	for _, f := range allFragments {
		if !seen.Has(f) {
			seen.Add(f)
			uniqueFragments = append(uniqueFragments, f)
		}
	}

	var contextContent string
	if len(uniqueFragments) > 0 {
		var err error
		contextContent, err = loader.LoadMultiple(uniqueFragments)
		if err != nil {
			return nil, fmt.Errorf("failed to load fragments: %w", err)
		}
		// Apply variable substitution (suppress warnings in MCP context)
		contextContent = substituteVariables(contextContent, profileVars, func(string) {})
	}

	return map[string]interface{}{
		"profiles":         profileNames,
		"fragments_loaded": uniqueFragments,
		"context":          contextContent,
	}, nil
}

func (s *mcpServer) toolListPrompts(args json.RawMessage) (interface{}, error) {
	var params struct {
		Query     string `json:"query"`
		SortBy    string `json:"sort_by"`
		SortOrder string `json:"sort_order"`
	}
	_ = json.Unmarshal(args, &params)

	loader := s.bundleLoader()

	prompts, err := loader.ListAllPrompts()
	if err != nil {
		return nil, err
	}

	type promptEntry struct {
		Name   string `json:"name"`
		Source string `json:"source"`
	}

	var result []promptEntry
	query := strings.ToLower(params.Query)
	for _, p := range prompts {
		// Filter by query if provided
		if query != "" && !strings.Contains(strings.ToLower(p.Name), query) {
			continue
		}
		result = append(result, promptEntry{
			Name:   p.Name,
			Source: p.Source,
		})
	}

	// Sort results
	reverse := params.SortOrder == "desc"
	sortSlice(result, func(i, j int) bool {
		cmp := strings.Compare(strings.ToLower(result[i].Name), strings.ToLower(result[j].Name))
		if reverse {
			return cmp > 0
		}
		return cmp < 0
	})

	return map[string]interface{}{
		"prompts": result,
		"count":   len(result),
	}, nil
}

func (s *mcpServer) toolGetPrompt(args json.RawMessage) (interface{}, error) {
	var params struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	loader := s.bundleLoader()

	prompt, err := loader.GetPrompt(params.Name)
	if err != nil {
		return nil, err
	}

	content := prompt.Content
	lines := strings.Split(content, "\n")
	var cleanedLines []string
	skipHeader := true
	for _, line := range lines {
		if skipHeader && strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		skipHeader = false
		cleanedLines = append(cleanedLines, line)
	}
	content = strings.TrimSpace(strings.Join(cleanedLines, "\n"))

	return map[string]interface{}{
		"name":    prompt.Name,
		"content": content,
	}, nil
}

func (s *mcpServer) toolSearchContent(args json.RawMessage) (interface{}, error) {
	var params struct {
		Query     string   `json:"query"`
		Types     []string `json:"types"`
		Tags      []string `json:"tags"`
		SortBy    string   `json:"sort_by"`
		SortOrder string   `json:"sort_order"`
		Limit     int      `json:"limit"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.Query == "" {
		return nil, fmt.Errorf("query is required")
	}
	if params.Limit <= 0 {
		params.Limit = 50
	}

	// Determine which types to search
	searchTypes := map[string]bool{
		"fragment":   true,
		"prompt":     true,
		"profile":    true,
		"mcp_server": true,
	}
	if len(params.Types) > 0 {
		searchTypes = make(map[string]bool)
		for _, t := range params.Types {
			searchTypes[t] = true
		}
	}

	type searchResult struct {
		Type   string   `json:"type"`
		Name   string   `json:"name"`
		Tags   []string `json:"tags,omitempty"`
		Source string   `json:"source,omitempty"`
		Match  string   `json:"match,omitempty"` // What matched (name, tag, description)
	}

	var results []searchResult
	query := strings.ToLower(params.Query)

	// Search fragments
	if searchTypes["fragment"] {
		loader := s.bundleLoader()
		var infos []bundles.ContentInfo
		var err error
		if len(params.Tags) > 0 {
			infos, err = loader.ListByTags(params.Tags)
		} else {
			infos, err = loader.ListAllFragments()
		}
		if err == nil {
			for _, info := range infos {
				matchType := ""
				if strings.Contains(strings.ToLower(info.Name), query) {
					matchType = "name"
				} else if containsTag(info.Tags, query) {
					matchType = "tag"
				}
				if matchType != "" {
					results = append(results, searchResult{
						Type:   "fragment",
						Name:   info.Name,
						Tags:   info.Tags,
						Source: info.Source,
						Match:  matchType,
					})
				}
			}
		}
	}

	// Search prompts
	if searchTypes["prompt"] {
		loader := s.bundleLoader()
		prompts, err := loader.ListAllPrompts()
		if err == nil {
			for _, p := range prompts {
				if strings.Contains(strings.ToLower(p.Name), query) {
					results = append(results, searchResult{
						Type:   "prompt",
						Name:   p.Name,
						Source: p.Source,
						Match:  "name",
					})
				}
			}
		}
	}

	// Search profiles
	if searchTypes["profile"] {
		for name, profile := range s.cfg.Profiles {
			matchType := ""
			if strings.Contains(strings.ToLower(name), query) {
				matchType = "name"
			} else if strings.Contains(strings.ToLower(profile.Description), query) {
				matchType = "description"
			} else if containsTag(profile.Tags, query) {
				matchType = "tag"
			}
			if matchType != "" {
				results = append(results, searchResult{
					Type:  "profile",
					Name:  name,
					Tags:  profile.Tags,
					Match: matchType,
				})
			}
		}
	}

	// Search MCP servers
	if searchTypes["mcp_server"] {
		cfg, err := config.Load()
		if err == nil {
			for name, srv := range cfg.MCP.Servers {
				if strings.Contains(strings.ToLower(name), query) ||
					strings.Contains(strings.ToLower(srv.Command), query) {
					results = append(results, searchResult{
						Type:   "mcp_server",
						Name:   name,
						Source: srv.Command,
						Match:  "name",
					})
				}
			}
		}
	}

	// Sort results
	sortBy := params.SortBy
	if sortBy == "" {
		sortBy = "relevance" // name matches first, then others
	}
	reverse := params.SortOrder == "desc"

	switch sortBy {
	case "name":
		sortSlice(results, func(i, j int) bool {
			cmp := strings.Compare(strings.ToLower(results[i].Name), strings.ToLower(results[j].Name))
			if reverse {
				return cmp > 0
			}
			return cmp < 0
		})
	case "type":
		sortSlice(results, func(i, j int) bool {
			cmp := strings.Compare(results[i].Type, results[j].Type)
			if reverse {
				return cmp > 0
			}
			return cmp < 0
		})
	case "relevance":
		// Name matches first, then tag/description matches
		sortSlice(results, func(i, j int) bool {
			scoreI := 0
			scoreJ := 0
			if results[i].Match == "name" {
				scoreI = 2
			} else if results[i].Match == "tag" {
				scoreI = 1
			}
			if results[j].Match == "name" {
				scoreJ = 2
			} else if results[j].Match == "tag" {
				scoreJ = 1
			}
			if reverse {
				return scoreI < scoreJ
			}
			return scoreI > scoreJ
		})
	}

	// Apply limit
	if len(results) > params.Limit {
		results = results[:params.Limit]
	}

	return map[string]interface{}{
		"results": results,
		"count":   len(results),
		"query":   params.Query,
	}, nil
}

// ============================================================================
// Remote tool implementations
// ============================================================================

func (s *mcpServer) toolListRemotes(ctx context.Context, args json.RawMessage) (interface{}, error) {
	registry, err := remote.NewRegistry("")
	if err != nil {
		return nil, fmt.Errorf("failed to load registry: %w", err)
	}

	remotes := registry.List()

	type remoteEntry struct {
		Name    string `json:"name"`
		URL     string `json:"url"`
		Version string `json:"version"`
	}

	var result []remoteEntry
	for _, r := range remotes {
		result = append(result, remoteEntry{
			Name:    r.Name,
			URL:     r.URL,
			Version: r.Version,
		})
	}

	return map[string]interface{}{
		"remotes": result,
		"count":   len(result),
	}, nil
}

func (s *mcpServer) toolDiscoverRemotes(ctx context.Context, args json.RawMessage) (interface{}, error) {
	var params struct {
		Query    string `json:"query"`
		Source   string `json:"source"`
		MinStars int    `json:"min_stars"`
	}
	_ = json.Unmarshal(args, &params)

	if params.Source == "" {
		params.Source = "all"
	}

	auth := remote.LoadAuth("")

	var forges []remote.ForgeType
	switch params.Source {
	case "github":
		forges = []remote.ForgeType{remote.ForgeGitHub}
	case "gitlab":
		forges = []remote.ForgeType{remote.ForgeGitLab}
	default:
		forges = []remote.ForgeType{remote.ForgeGitHub, remote.ForgeGitLab}
	}

	var wg sync.WaitGroup
	resultsCh := make(chan []remote.RepoInfo, len(forges))
	errorsCh := make(chan error, len(forges))

	for _, forge := range forges {
		wg.Add(1)
		go func(f remote.ForgeType) {
			defer wg.Done()

			var fetcher remote.Fetcher
			var err error

			switch f {
			case remote.ForgeGitHub:
				fetcher = remote.NewGitHubFetcher(auth.GitHub)
			case remote.ForgeGitLab:
				fetcher, err = remote.NewGitLabFetcher("", auth.GitLab)
				if err != nil {
					errorsCh <- fmt.Errorf("GitLab: %w", err)
					return
				}
			}

			repos, err := fetcher.SearchRepos(ctx, params.Query, 30)
			if err != nil {
				errorsCh <- fmt.Errorf("%s: %w", f, err)
				return
			}

			filtered := repos[:0]
			for _, r := range repos {
				if r.Stars >= params.MinStars {
					filtered = append(filtered, r)
				}
			}

			resultsCh <- filtered
		}(forge)
	}

	wg.Wait()
	close(resultsCh)
	close(errorsCh)

	var allRepos []remote.RepoInfo
	for repos := range resultsCh {
		allRepos = append(allRepos, repos...)
	}

	var errors []string
	for err := range errorsCh {
		errors = append(errors, err.Error())
	}

	type repoEntry struct {
		Owner       string `json:"owner"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Stars       int    `json:"stars"`
		URL         string `json:"url"`
		Forge       string `json:"forge"`
		AddCommand  string `json:"add_command"`
	}

	var result []repoEntry
	for _, r := range allRepos {
		result = append(result, repoEntry{
			Owner:       r.Owner,
			Name:        r.Name,
			Description: r.Description,
			Stars:       r.Stars,
			URL:         r.URL,
			Forge:       string(r.Forge),
			AddCommand:  fmt.Sprintf("scm remote add %s %s/%s", r.Owner, r.Owner, r.Name),
		})
	}

	return map[string]interface{}{
		"repositories": result,
		"count":        len(result),
		"errors":       errors,
	}, nil
}

func (s *mcpServer) toolBrowseRemote(ctx context.Context, args json.RawMessage) (interface{}, error) {
	var params struct {
		Remote   string `json:"remote"`
		ItemType string `json:"item_type"`
		Path     string `json:"path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.Remote == "" {
		return nil, fmt.Errorf("remote is required")
	}

	registry, err := remote.NewRegistry("")
	if err != nil {
		return nil, fmt.Errorf("failed to load registry: %w", err)
	}

	rem, err := registry.Get(params.Remote)
	if err != nil {
		return nil, err
	}

	auth := remote.LoadAuth("")
	fetcher, err := remote.NewFetcher(rem.URL, auth)
	if err != nil {
		return nil, fmt.Errorf("failed to create fetcher: %w", err)
	}

	owner, repo, err := remote.ParseRepoURL(rem.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid remote URL: %w", err)
	}

	// Determine which types to list (only bundles and profiles supported)
	var itemTypes []remote.ItemType
	switch params.ItemType {
	case "bundle":
		itemTypes = []remote.ItemType{remote.ItemTypeBundle}
	case "profile":
		itemTypes = []remote.ItemType{remote.ItemTypeProfile}
	default:
		itemTypes = []remote.ItemType{remote.ItemTypeBundle, remote.ItemTypeProfile}
	}

	type itemEntry struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		Path     string `json:"path"`
		IsDir    bool   `json:"is_dir,omitempty"`
		PullRef  string `json:"pull_ref"`
	}

	var items []itemEntry

	for _, itemType := range itemTypes {
		basePath := fmt.Sprintf("scm/%s/%s", rem.Version, itemType.DirName())
		if params.Path != "" {
			basePath = filepath.Join(basePath, params.Path)
		}

		entries, err := fetcher.ListDir(ctx, owner, repo, basePath, "")
		if err != nil {
			continue // Directory might not exist for this type
		}

		for _, e := range entries {
			name := e.Name
			if !e.IsDir && strings.HasSuffix(name, ".yaml") {
				name = strings.TrimSuffix(name, ".yaml")
			}

			pullPath := name
			if params.Path != "" {
				pullPath = params.Path + "/" + name
			}

			items = append(items, itemEntry{
				Name:    name,
				Type:    string(itemType),
				Path:    pullPath,
				IsDir:   e.IsDir,
				PullRef: fmt.Sprintf("%s/%s", params.Remote, pullPath),
			})
		}
	}

	return map[string]interface{}{
		"remote": params.Remote,
		"url":    rem.URL,
		"items":  items,
		"count":  len(items),
	}, nil
}

func (s *mcpServer) toolPreviewRemote(ctx context.Context, args json.RawMessage) (interface{}, error) {
	var params struct {
		Reference string `json:"reference"`
		ItemType  string `json:"item_type"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.Reference == "" {
		return nil, fmt.Errorf("reference is required")
	}
	if params.ItemType == "" {
		return nil, fmt.Errorf("item_type is required")
	}

	var itemType remote.ItemType
	switch params.ItemType {
	case "bundle":
		itemType = remote.ItemTypeBundle
	case "profile":
		itemType = remote.ItemTypeProfile
	default:
		return nil, fmt.Errorf("invalid item_type: %s (only bundle and profile supported)", params.ItemType)
	}

	ref, err := remote.ParseReference(params.Reference)
	if err != nil {
		return nil, fmt.Errorf("invalid reference: %w", err)
	}

	registry, err := remote.NewRegistry("")
	if err != nil {
		return nil, fmt.Errorf("failed to load registry: %w", err)
	}

	rem, err := registry.Get(ref.Remote)
	if err != nil {
		return nil, err
	}

	auth := remote.LoadAuth("")
	fetcher, err := remote.NewFetcher(rem.URL, auth)
	if err != nil {
		return nil, fmt.Errorf("failed to create fetcher: %w", err)
	}

	owner, repo, err := remote.ParseRepoURL(rem.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid remote URL: %w", err)
	}

	// Resolve ref to SHA
	gitRef := ref.GitRef
	if gitRef == "" {
		gitRef, err = fetcher.GetDefaultBranch(ctx, owner, repo)
		if err != nil {
			return nil, fmt.Errorf("failed to get default branch: %w", err)
		}
	}

	sha, err := fetcher.ResolveRef(ctx, owner, repo, gitRef)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve ref '%s': %w", gitRef, err)
	}

	// Fetch content
	filePath := ref.BuildFilePath(itemType, rem.Version)
	content, err := fetcher.FetchFile(ctx, owner, repo, filePath, sha)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch: %w", err)
	}

	// Generate pull token (reference pinned to SHA)
	pullToken := fmt.Sprintf("%s/%s@%s", ref.Remote, ref.Path, sha)

	// Store pending pull
	s.pullMu.Lock()
	s.pendingPulls[pullToken] = &pendingPull{
		Reference: pullToken,
		ItemType:  itemType,
		Content:   content,
		SHA:       sha,
		RemoteURL: rem.URL,
	}
	s.pullMu.Unlock()

	shortSHA := sha
	if len(sha) > 7 {
		shortSHA = sha[:7]
	}

	return map[string]interface{}{
		"reference":  params.Reference,
		"item_type":  params.ItemType,
		"sha":        shortSHA,
		"full_sha":   sha,
		"source_url": rem.URL,
		"file_path":  filePath,
		"content":    string(content),
		"pull_token": pullToken,
		"warning":    "REVIEW THIS CONTENT CAREFULLY. Malicious prompts can override AI safety guidelines, exfiltrate data, or execute unintended actions. Use confirm_pull with the pull_token to install.",
	}, nil
}

func (s *mcpServer) toolConfirmPull(ctx context.Context, args json.RawMessage) (interface{}, error) {
	var params struct {
		PullToken string `json:"pull_token"`
		Local     bool   `json:"local"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.PullToken == "" {
		return nil, fmt.Errorf("pull_token is required")
	}

	// Get pending pull
	s.pullMu.Lock()
	pending, ok := s.pendingPulls[params.PullToken]
	if ok {
		delete(s.pendingPulls, params.PullToken)
	}
	s.pullMu.Unlock()

	if !ok {
		return nil, fmt.Errorf("invalid or expired pull_token: token must be obtained from preview_remote")
	}

	// Parse the reference to get path info
	ref, err := remote.ParseReference(pending.Reference)
	if err != nil {
		return nil, fmt.Errorf("invalid reference in token: %w", err)
	}

	// Determine local path
	baseDir := ".scm"
	if !params.Local {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			baseDir = filepath.Join(homeDir, ".scm")
		}
	}

	localPath := ref.LocalPath(baseDir, pending.ItemType)

	// Check for existing file
	overwritten := false
	if _, err := os.Stat(localPath); err == nil {
		overwritten = true
	}

	// Write file
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(localPath, pending.Content, 0644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	shortSHA := pending.SHA
	if len(shortSHA) > 7 {
		shortSHA = shortSHA[:7]
	}

	action := "installed"
	if overwritten {
		action = "updated"
	}

	return map[string]interface{}{
		"status":      action,
		"reference":   pending.Reference,
		"item_type":   string(pending.ItemType),
		"local_path":  localPath,
		"sha":         shortSHA,
		"overwritten": overwritten,
	}, nil
}

// ============================================================================
// Profile management tools
// ============================================================================

func (s *mcpServer) toolCreateProfile(args json.RawMessage) (interface{}, error) {
	var params struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Parents     []string `json:"parents"`
		Fragments   []string `json:"fragments"`
		Tags        []string `json:"tags"`
		Generators  []string `json:"generators"`
		Default     bool     `json:"default"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	// Reload config to ensure freshness
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	if _, exists := cfg.Profiles[params.Name]; exists {
		return nil, fmt.Errorf("profile %q already exists", params.Name)
	}

	// Validate parents exist
	for _, parent := range params.Parents {
		if _, exists := cfg.Profiles[parent]; !exists {
			return nil, fmt.Errorf("parent profile %q not found", parent)
		}
	}

	if cfg.Profiles == nil {
		cfg.Profiles = make(map[string]config.Profile)
	}

	cfg.Profiles[params.Name] = config.Profile{
		Description: params.Description,
		Parents:     params.Parents,
		Tags:        params.Tags,
		Fragments:   params.Fragments,
		Generators:  params.Generators,
	}

	// Set as default if requested
	if params.Default {
		cfg.Defaults.Profile = params.Name
	}

	if err := cfg.Save(); err != nil {
		return nil, fmt.Errorf("failed to save config: %w", err)
	}

	// Update local config reference
	s.cfg = cfg

	return map[string]interface{}{
		"status":  "created",
		"profile": params.Name,
	}, nil
}

func (s *mcpServer) toolUpdateProfile(args json.RawMessage) (interface{}, error) {
	var params struct {
		Name             string   `json:"name"`
		Description      *string  `json:"description"`
		AddParents       []string `json:"add_parents"`
		RemoveParents    []string `json:"remove_parents"`
		AddFragments     []string `json:"add_fragments"`
		RemoveFragments  []string `json:"remove_fragments"`
		AddTags          []string `json:"add_tags"`
		RemoveTags       []string `json:"remove_tags"`
		AddGenerators    []string `json:"add_generators"`
		RemoveGenerators []string `json:"remove_generators"`
		Default          *bool    `json:"default"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	// Reload config
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	profile, exists := cfg.Profiles[params.Name]
	if !exists {
		return nil, fmt.Errorf("profile %q not found", params.Name)
	}

	changes := []string{}

	// Update description
	if params.Description != nil {
		profile.Description = *params.Description
		changes = append(changes, "updated description")
	}

	// Update default flag
	if params.Default != nil {
		if *params.Default {
			cfg.Defaults.Profile = params.Name
			changes = append(changes, "set as default")
		} else if cfg.Defaults.Profile == params.Name {
			cfg.Defaults.Profile = ""
			changes = append(changes, "unset default")
		}
	}

	// Add parents
	for _, parent := range params.AddParents {
		if _, exists := cfg.Profiles[parent]; !exists {
			return nil, fmt.Errorf("parent profile %q not found", parent)
		}
		if !contains(profile.Parents, parent) {
			profile.Parents = append(profile.Parents, parent)
			changes = append(changes, fmt.Sprintf("added parent: %s", parent))
		}
	}

	// Remove parents
	for _, parent := range params.RemoveParents {
		if idx := indexOf(profile.Parents, parent); idx >= 0 {
			profile.Parents = append(profile.Parents[:idx], profile.Parents[idx+1:]...)
			changes = append(changes, fmt.Sprintf("removed parent: %s", parent))
		}
	}

	// Add fragments
	for _, f := range params.AddFragments {
		if !contains(profile.Fragments, f) {
			profile.Fragments = append(profile.Fragments, f)
			changes = append(changes, fmt.Sprintf("added fragment: %s", f))
		}
	}

	// Remove fragments
	for _, f := range params.RemoveFragments {
		if idx := indexOf(profile.Fragments, f); idx >= 0 {
			profile.Fragments = append(profile.Fragments[:idx], profile.Fragments[idx+1:]...)
			changes = append(changes, fmt.Sprintf("removed fragment: %s", f))
		}
	}

	// Add tags
	for _, t := range params.AddTags {
		if !contains(profile.Tags, t) {
			profile.Tags = append(profile.Tags, t)
			changes = append(changes, fmt.Sprintf("added tag: %s", t))
		}
	}

	// Remove tags
	for _, t := range params.RemoveTags {
		if idx := indexOf(profile.Tags, t); idx >= 0 {
			profile.Tags = append(profile.Tags[:idx], profile.Tags[idx+1:]...)
			changes = append(changes, fmt.Sprintf("removed tag: %s", t))
		}
	}

	// Add generators
	for _, g := range params.AddGenerators {
		if !contains(profile.Generators, g) {
			profile.Generators = append(profile.Generators, g)
			changes = append(changes, fmt.Sprintf("added generator: %s", g))
		}
	}

	// Remove generators
	for _, g := range params.RemoveGenerators {
		if idx := indexOf(profile.Generators, g); idx >= 0 {
			profile.Generators = append(profile.Generators[:idx], profile.Generators[idx+1:]...)
			changes = append(changes, fmt.Sprintf("removed generator: %s", g))
		}
	}

	if len(changes) == 0 {
		return map[string]interface{}{
			"status":  "no_changes",
			"profile": params.Name,
		}, nil
	}

	cfg.Profiles[params.Name] = profile
	if err := cfg.Save(); err != nil {
		return nil, fmt.Errorf("failed to save config: %w", err)
	}

	s.cfg = cfg

	return map[string]interface{}{
		"status":  "updated",
		"profile": params.Name,
		"changes": changes,
	}, nil
}

func (s *mcpServer) toolDeleteProfile(args json.RawMessage) (interface{}, error) {
	var params struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	if _, exists := cfg.Profiles[params.Name]; !exists {
		return nil, fmt.Errorf("profile %q not found", params.Name)
	}

	delete(cfg.Profiles, params.Name)

	if err := cfg.Save(); err != nil {
		return nil, fmt.Errorf("failed to save config: %w", err)
	}

	s.cfg = cfg

	return map[string]interface{}{
		"status":  "deleted",
		"profile": params.Name,
	}, nil
}

// ============================================================================
// Fragment management tools
// ============================================================================

func (s *mcpServer) toolCreateFragment(args json.RawMessage) (interface{}, error) {
	var params struct {
		Name    string   `json:"name"`
		Content string   `json:"content"`
		Tags    []string `json:"tags"`
		Version string   `json:"version"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if params.Content == "" {
		return nil, fmt.Errorf("content is required")
	}

	if params.Version == "" {
		params.Version = "1.0"
	}

	// Build YAML content
	frag := map[string]interface{}{
		"version": params.Version,
		"content": params.Content,
	}
	if len(params.Tags) > 0 {
		frag["tags"] = params.Tags
	}

	yamlContent, err := yaml.Marshal(frag)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal fragment: %w", err)
	}

	// Determine path - use project .scm directory
	fragmentDir := filepath.Join(".scm", config.ContextFragmentsDir)
	if err := os.MkdirAll(fragmentDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create fragment directory: %w", err)
	}

	fragmentPath := filepath.Join(fragmentDir, params.Name+".yaml")

	// Check if exists
	overwritten := false
	if _, err := os.Stat(fragmentPath); err == nil {
		overwritten = true
	}

	if err := os.WriteFile(fragmentPath, yamlContent, 0644); err != nil {
		return nil, fmt.Errorf("failed to write fragment: %w", err)
	}

	action := "created"
	if overwritten {
		action = "updated"
	}

	return map[string]interface{}{
		"status":      action,
		"fragment":    params.Name,
		"path":        fragmentPath,
		"overwritten": overwritten,
	}, nil
}

func (s *mcpServer) toolDeleteFragment(args json.RawMessage) (interface{}, error) {
	var params struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	// Fragments are now part of bundles and cannot be deleted individually
	return nil, fmt.Errorf("individual fragments cannot be deleted; they are part of bundles. Use bundle management instead")
}

// ============================================================================
// Hooks management tools
// ============================================================================

func (s *mcpServer) toolApplyHooks(ctx context.Context, args json.RawMessage) (interface{}, error) {
	var params struct {
		Backend           string `json:"backend"`
		RegenerateContext *bool  `json:"regenerate_context"`
	}
	_ = json.Unmarshal(args, &params)

	if params.Backend == "" {
		params.Backend = "all"
	}

	regenerate := true
	if params.RegenerateContext != nil {
		regenerate = *params.RegenerateContext
	}

	// Reload config
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Determine work directory
	workDir := "."
	if root, err := gitutil.FindRoot("."); err == nil {
		workDir = root
	}

	// Ensure SCM symlink exists
	if _, err := backends.EnsureSCMSymlink(workDir); err != nil {
		return nil, fmt.Errorf("failed to create scm symlink: %w", err)
	}

	var contextHash string
	if regenerate {
		// Load fragments from default profiles using bundles
		loader := bundles.NewLoader(cfg.GetBundleDirs(), cfg.Defaults.ShouldUseDistilled())
		var allFragments []string

		for _, profileName := range cfg.GetDefaultProfiles() {
			profile, err := config.ResolveProfile(cfg.Profiles, profileName)
			if err != nil {
				continue
			}

			if len(profile.Tags) > 0 {
				taggedInfos, _ := loader.ListByTags(profile.Tags)
				for _, info := range taggedInfos {
					allFragments = append(allFragments, info.Name)
				}
			}

			allFragments = append(allFragments, profile.Fragments...)
		}

		// Dedupe
		allFragments = config.DedupeStrings(allFragments)

		// Load and write context
		if len(allFragments) > 0 {
			var backendFrags []*backends.Fragment
			for _, name := range allFragments {
				content, err := loader.GetFragment(name)
				if err != nil {
					continue
				}
				backendFrags = append(backendFrags, &backends.Fragment{
					Name:    content.Name,
					Content: content.Content,
				})
			}
			if len(backendFrags) > 0 {
				contextHash, _ = backends.WriteContextFile(workDir, backendFrags)
			}
		}
	}

	applied := []string{}

	// Load MCP servers from profile bundles
	bundleMCP := cfg.ResolveBundleMCPServers()

	// Apply to backends
	if params.Backend == "all" || params.Backend == "claude-code" {
		hooksCfg := &cfg.Hooks
		if contextHash != "" {
			hooksCfg.Unified.SessionStart = append(hooksCfg.Unified.SessionStart, backends.NewContextInjectionHook(contextHash))
		}
		if err := backends.WriteSettings("claude-code", hooksCfg, &cfg.MCP, bundleMCP, workDir); err != nil {
			return nil, fmt.Errorf("failed to apply claude-code settings: %w", err)
		}
		applied = append(applied, "claude-code")
	}

	if params.Backend == "all" || params.Backend == "gemini" {
		hooksCfg := &cfg.Hooks
		if contextHash != "" {
			hooksCfg.Unified.SessionStart = append(hooksCfg.Unified.SessionStart, backends.NewContextInjectionHook(contextHash))
		}
		if err := backends.WriteSettings("gemini", hooksCfg, &cfg.MCP, bundleMCP, workDir); err != nil {
			return nil, fmt.Errorf("failed to apply gemini settings: %w", err)
		}
		applied = append(applied, "gemini")
	}

	result := map[string]interface{}{
		"status":   "applied",
		"backends": applied,
	}
	if contextHash != "" {
		result["context_hash"] = contextHash
	}

	return result, nil
}

// ============================================================================
// Remote management tools
// ============================================================================

func (s *mcpServer) toolAddRemote(ctx context.Context, args json.RawMessage) (interface{}, error) {
	var params struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if params.URL == "" {
		return nil, fmt.Errorf("url is required")
	}

	registry, err := remote.NewRegistry("")
	if err != nil {
		return nil, fmt.Errorf("failed to initialize registry: %w", err)
	}

	if err := registry.Add(params.Name, params.URL); err != nil {
		return nil, err
	}

	// Verify the remote
	auth := remote.LoadAuth("")
	fetcher, err := registry.GetFetcher(params.Name, auth)
	if err != nil {
		registry.Remove(params.Name)
		return nil, fmt.Errorf("failed to create fetcher: %w", err)
	}

	owner, repo, err := remote.ParseRepoURL(params.URL)
	if err != nil {
		registry.Remove(params.Name)
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	valid, _ := fetcher.ValidateRepo(ctx, owner, repo)

	rem, _ := registry.Get(params.Name)

	result := map[string]interface{}{
		"status": "added",
		"name":   params.Name,
		"url":    rem.URL,
	}
	if !valid {
		result["warning"] = "repository does not have scm/v1/ directory structure"
	}

	return result, nil
}

func (s *mcpServer) toolRemoveRemote(ctx context.Context, args json.RawMessage) (interface{}, error) {
	var params struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	registry, err := remote.NewRegistry("")
	if err != nil {
		return nil, fmt.Errorf("failed to initialize registry: %w", err)
	}

	if err := registry.Remove(params.Name); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"status": "removed",
		"name":   params.Name,
	}, nil
}

// ============================================================================
// Lockfile management tools
// ============================================================================

func (s *mcpServer) toolLockDependencies(ctx context.Context, args json.RawMessage) (interface{}, error) {
	baseDir := ".scm"

	lockManager := remote.NewLockfileManager(baseDir)
	lockfile := &remote.Lockfile{
		Version:  1,
		Bundles:  make(map[string]remote.LockEntry),
		Profiles: make(map[string]remote.LockEntry),
	}

	itemCount := 0

	// Scan for installed items (bundles and profiles only)
	for _, itemType := range []remote.ItemType{
		remote.ItemTypeBundle,
		remote.ItemTypeProfile,
	} {
		var dirName string
		switch itemType {
		case remote.ItemTypeBundle:
			dirName = "bundles"
		case remote.ItemTypeProfile:
			dirName = "profiles"
		}

		itemDir := filepath.Join(baseDir, dirName)
		entries, err := os.ReadDir(itemDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			remoteName := entry.Name()
			remoteDir := filepath.Join(itemDir, remoteName)

			files, _ := filepath.Glob(filepath.Join(remoteDir, "**", "*.yaml"))
			rootFiles, _ := filepath.Glob(filepath.Join(remoteDir, "*.yaml"))
			files = append(files, rootFiles...)

			for _, file := range files {
				content, err := os.ReadFile(file)
				if err != nil {
					continue
				}

				var meta struct {
					Source remote.SourceMeta `yaml:"_source"`
				}
				if err := yaml.Unmarshal(content, &meta); err != nil {
					continue
				}

				if meta.Source.SHA == "" {
					continue
				}

				relPath, _ := filepath.Rel(filepath.Join(itemDir, remoteName), file)
				name := strings.TrimSuffix(relPath, ".yaml")
				ref := fmt.Sprintf("%s/%s", remoteName, name)

				lockEntry := remote.LockEntry{
					SHA:        meta.Source.SHA,
					URL:        meta.Source.URL,
					SCMVersion: meta.Source.Version,
					FetchedAt:  meta.Source.FetchedAt,
				}

				lockfile.AddEntry(itemType, ref, lockEntry)
				itemCount++
			}
		}
	}

	if itemCount == 0 {
		return map[string]interface{}{
			"status":  "empty",
			"message": "No remote items with source metadata found",
		}, nil
	}

	if err := lockManager.Save(lockfile); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"status":     "generated",
		"path":       lockManager.Path(),
		"item_count": itemCount,
	}, nil
}

func (s *mcpServer) toolInstallDependencies(ctx context.Context, args json.RawMessage) (interface{}, error) {
	var params struct {
		Force bool `json:"force"`
	}
	_ = json.Unmarshal(args, &params)

	lockManager := remote.NewLockfileManager(".scm")
	lockfile, err := lockManager.Load()
	if err != nil {
		return nil, err
	}

	if lockfile.IsEmpty() {
		return map[string]interface{}{
			"status":  "empty",
			"message": "No entries in lockfile",
		}, nil
	}

	registry, err := remote.NewRegistry("")
	if err != nil {
		return nil, fmt.Errorf("failed to initialize registry: %w", err)
	}

	auth := remote.LoadAuth("")
	puller := remote.NewPuller(registry, auth)

	entries := lockfile.AllEntries()
	installed := 0
	failed := 0
	var errors []string

	for _, e := range entries {
		ref := fmt.Sprintf("%s@%s", e.Ref, e.Entry.SHA[:7])

		opts := remote.PullOptions{
			Force:    params.Force,
			ItemType: e.Type,
		}

		_, err := puller.Pull(ctx, ref, opts)
		if err != nil {
			failed++
			errors = append(errors, fmt.Sprintf("%s: %v", e.Ref, err))
			continue
		}
		installed++
	}

	result := map[string]interface{}{
		"status":    "completed",
		"installed": installed,
		"failed":    failed,
		"total":     len(entries),
	}
	if len(errors) > 0 {
		result["errors"] = errors
	}

	return result, nil
}

func (s *mcpServer) toolCheckOutdated(ctx context.Context, args json.RawMessage) (interface{}, error) {
	lockManager := remote.NewLockfileManager(".scm")
	lockfile, err := lockManager.Load()
	if err != nil {
		return nil, err
	}

	if lockfile.IsEmpty() {
		return map[string]interface{}{
			"status":  "empty",
			"message": "No entries in lockfile",
		}, nil
	}

	registry, err := remote.NewRegistry("")
	if err != nil {
		return nil, fmt.Errorf("failed to initialize registry: %w", err)
	}

	auth := remote.LoadAuth("")
	entries := lockfile.AllEntries()

	type outdatedItem struct {
		Type      string `json:"type"`
		Reference string `json:"reference"`
		LockedSHA string `json:"locked_sha"`
		LatestSHA string `json:"latest_sha"`
	}

	var outdated []outdatedItem

	for _, e := range entries {
		ref, err := remote.ParseReference(e.Ref)
		if err != nil {
			continue
		}

		rem, err := registry.Get(ref.Remote)
		if err != nil {
			continue
		}

		fetcher, err := remote.NewFetcher(rem.URL, auth)
		if err != nil {
			continue
		}

		owner, repo, err := remote.ParseRepoURL(rem.URL)
		if err != nil {
			continue
		}

		branch, err := fetcher.GetDefaultBranch(ctx, owner, repo)
		if err != nil {
			continue
		}

		latestSHA, err := fetcher.ResolveRef(ctx, owner, repo, branch)
		if err != nil {
			continue
		}

		if latestSHA != e.Entry.SHA {
			lockedShort := e.Entry.SHA
			if len(lockedShort) > 7 {
				lockedShort = lockedShort[:7]
			}
			latestShort := latestSHA
			if len(latestShort) > 7 {
				latestShort = latestShort[:7]
			}

			outdated = append(outdated, outdatedItem{
				Type:      string(e.Type),
				Reference: e.Ref,
				LockedSHA: lockedShort,
				LatestSHA: latestShort,
			})
		}
	}

	if len(outdated) == 0 {
		return map[string]interface{}{
			"status":  "up_to_date",
			"message": "All items are up to date",
		}, nil
	}

	return map[string]interface{}{
		"status":   "outdated",
		"count":    len(outdated),
		"items":    outdated,
		"total":    len(entries),
	}, nil
}

// ============================================================================
// MCP server management tools
// ============================================================================

func (s *mcpServer) toolListMCPServers(ctx context.Context, args json.RawMessage) (interface{}, error) {
	var params struct {
		Query     string `json:"query"`
		SortBy    string `json:"sort_by"`
		SortOrder string `json:"sort_order"`
	}
	_ = json.Unmarshal(args, &params)

	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	type serverEntry struct {
		Name    string   `json:"name"`
		Command string   `json:"command"`
		Args    []string `json:"args,omitempty"`
		Backend string   `json:"backend"`
	}

	var servers []serverEntry
	query := strings.ToLower(params.Query)

	// Unified servers
	for name, srv := range cfg.MCP.Servers {
		if query != "" && !strings.Contains(strings.ToLower(name), query) &&
			!strings.Contains(strings.ToLower(srv.Command), query) {
			continue
		}
		servers = append(servers, serverEntry{
			Name:    name,
			Command: srv.Command,
			Args:    srv.Args,
			Backend: "unified",
		})
	}

	// Backend-specific servers
	for backend, backendServers := range cfg.MCP.Plugins {
		for name, srv := range backendServers {
			if query != "" && !strings.Contains(strings.ToLower(name), query) &&
				!strings.Contains(strings.ToLower(srv.Command), query) {
				continue
			}
			servers = append(servers, serverEntry{
				Name:    name,
				Command: srv.Command,
				Args:    srv.Args,
				Backend: backend,
			})
		}
	}

	// Sort results
	sortBy := params.SortBy
	if sortBy == "" {
		sortBy = "name"
	}
	reverse := params.SortOrder == "desc"

	switch sortBy {
	case "name":
		sortSlice(servers, func(i, j int) bool {
			cmp := strings.Compare(strings.ToLower(servers[i].Name), strings.ToLower(servers[j].Name))
			if reverse {
				return cmp > 0
			}
			return cmp < 0
		})
	case "command":
		sortSlice(servers, func(i, j int) bool {
			cmp := strings.Compare(strings.ToLower(servers[i].Command), strings.ToLower(servers[j].Command))
			if reverse {
				return cmp > 0
			}
			return cmp < 0
		})
	}

	return map[string]interface{}{
		"servers":       servers,
		"count":         len(servers),
		"auto_register": cfg.MCP.ShouldAutoRegisterSCM(),
	}, nil
}

func (s *mcpServer) toolAddMCPServer(ctx context.Context, args json.RawMessage) (interface{}, error) {
	var params struct {
		Name    string   `json:"name"`
		Command string   `json:"command"`
		Args    []string `json:"args"`
		Backend string   `json:"backend"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if params.Command == "" {
		return nil, fmt.Errorf("command is required")
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	server := config.MCPServer{
		Command: params.Command,
		Args:    params.Args,
	}

	if params.Backend == "" || params.Backend == "unified" {
		if cfg.MCP.Servers == nil {
			cfg.MCP.Servers = make(map[string]config.MCPServer)
		}
		if _, exists := cfg.MCP.Servers[params.Name]; exists {
			return nil, fmt.Errorf("MCP server %q already exists", params.Name)
		}
		cfg.MCP.Servers[params.Name] = server
	} else {
		if cfg.MCP.Plugins == nil {
			cfg.MCP.Plugins = make(map[string]map[string]config.MCPServer)
		}
		if cfg.MCP.Plugins[params.Backend] == nil {
			cfg.MCP.Plugins[params.Backend] = make(map[string]config.MCPServer)
		}
		if _, exists := cfg.MCP.Plugins[params.Backend][params.Name]; exists {
			return nil, fmt.Errorf("MCP server %q already exists for backend %s", params.Name, params.Backend)
		}
		cfg.MCP.Plugins[params.Backend][params.Name] = server
	}

	if err := cfg.Save(); err != nil {
		return nil, fmt.Errorf("failed to save config: %w", err)
	}

	s.cfg = cfg

	scope := "unified"
	if params.Backend != "" && params.Backend != "unified" {
		scope = params.Backend
	}

	return map[string]interface{}{
		"status":  "added",
		"name":    params.Name,
		"command": params.Command,
		"backend": scope,
		"note":    "Run apply_hooks to inject into backend settings",
	}, nil
}

func (s *mcpServer) toolRemoveMCPServer(ctx context.Context, args json.RawMessage) (interface{}, error) {
	var params struct {
		Name    string `json:"name"`
		Backend string `json:"backend"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	removed := false
	removedFrom := []string{}

	// Remove from unified if no specific backend or if unified specified
	if params.Backend == "" || params.Backend == "unified" {
		if _, exists := cfg.MCP.Servers[params.Name]; exists {
			delete(cfg.MCP.Servers, params.Name)
			removed = true
			removedFrom = append(removedFrom, "unified")
		}
	}

	// Remove from backend-specific
	if params.Backend != "" && params.Backend != "unified" {
		if backendServers, ok := cfg.MCP.Plugins[params.Backend]; ok {
			if _, exists := backendServers[params.Name]; exists {
				delete(backendServers, params.Name)
				removed = true
				removedFrom = append(removedFrom, params.Backend)
			}
		}
	} else if params.Backend == "" {
		// If no backend specified, try to remove from all backends
		for backend, servers := range cfg.MCP.Plugins {
			if _, exists := servers[params.Name]; exists {
				delete(servers, params.Name)
				removed = true
				removedFrom = append(removedFrom, backend)
			}
		}
	}

	if !removed {
		return nil, fmt.Errorf("MCP server %q not found", params.Name)
	}

	if err := cfg.Save(); err != nil {
		return nil, fmt.Errorf("failed to save config: %w", err)
	}

	s.cfg = cfg

	return map[string]interface{}{
		"status":       "removed",
		"name":         params.Name,
		"removed_from": removedFrom,
		"note":         "Run apply_hooks to update backend settings",
	}, nil
}

func (s *mcpServer) toolSetMCPAutoRegister(ctx context.Context, args json.RawMessage) (interface{}, error) {
	var params struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	cfg.MCP.AutoRegisterSCM = &params.Enabled

	if err := cfg.Save(); err != nil {
		return nil, fmt.Errorf("failed to save config: %w", err)
	}

	s.cfg = cfg

	return map[string]interface{}{
		"status":        "updated",
		"auto_register": params.Enabled,
		"note":          "Run apply_hooks to update backend settings",
	}, nil
}

// ============================================================================
// Helper functions
// ============================================================================

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func indexOf(slice []string, item string) int {
	for i, s := range slice {
		if s == item {
			return i
		}
	}
	return -1
}
