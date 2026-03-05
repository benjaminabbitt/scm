package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/benjaminabbitt/scm/internal/config"
)

var mcpServersCmd = &cobra.Command{
	Use:   "mcp-servers",
	Short: "Manage MCP (Model Context Protocol) server configurations",
	Long: `Manage MCP server configurations that are injected into backend settings.

MCP servers extend AI agent capabilities by providing additional tools and resources.
SCM can manage these configurations and inject them into backend-specific settings
files (.claude/settings.json, .gemini/settings.json, etc.).

By default, SCM auto-registers its own MCP server. You can disable this with
'scm mcp-servers auto-register --disable'.`,
}

var mcpServersListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List configured MCP servers",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := GetConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Collect all servers
		type serverInfo struct {
			Name    string
			Command string
			Args    []string
			Backend string // "unified" or backend name
		}
		var servers []serverInfo

		// Unified servers
		for name, srv := range cfg.MCP.Servers {
			servers = append(servers, serverInfo{
				Name:    name,
				Command: srv.Command,
				Args:    srv.Args,
				Backend: "unified",
			})
		}

		// Backend-specific servers
		for backend, backendServers := range cfg.MCP.Plugins {
			for name, srv := range backendServers {
				servers = append(servers, serverInfo{
					Name:    name,
					Command: srv.Command,
					Args:    srv.Args,
					Backend: backend,
				})
			}
		}

		if len(servers) == 0 {
			fmt.Println("No MCP servers configured.")
			fmt.Println()
			fmt.Printf("Auto-register SCM MCP server: %v\n", cfg.MCP.ShouldAutoRegisterSCM())
			fmt.Println("\nUse 'scm mcp-servers add <name> --command <cmd>' to add one.")
			return nil
		}

		// Sort by name
		sort.Slice(servers, func(i, j int) bool {
			return servers[i].Name < servers[j].Name
		})

		fmt.Println("MCP Servers:")
		for _, srv := range servers {
			fmt.Printf("  %s\n", srv.Name)
			fmt.Printf("    Command: %s\n", srv.Command)
			if len(srv.Args) > 0 {
				fmt.Printf("    Args: %s\n", strings.Join(srv.Args, " "))
			}
			fmt.Printf("    Scope: %s\n", srv.Backend)
		}

		fmt.Printf("\nAuto-register SCM MCP server: %v\n", cfg.MCP.ShouldAutoRegisterSCM())
		return nil
	},
}

var (
	mcpServersAddCommand string
	mcpServersAddArgs    []string
	mcpServersAddBackend string
)

var mcpServersAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add an MCP server configuration",
	Long: `Add an MCP server to be injected into backend settings.

Examples:
  scm mcp-servers add my-server --command "npx my-mcp-server"
  scm mcp-servers add tools --command "python" --args "-m,mcp_tools"
  scm mcp-servers add claude-only --command "./server" --backend claude-code`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		if mcpServersAddCommand == "" {
			return fmt.Errorf("--command is required")
		}

		cfg, err := GetConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		server := config.MCPServer{
			Command: mcpServersAddCommand,
			Args:    mcpServersAddArgs,
		}

		if mcpServersAddBackend == "" || mcpServersAddBackend == "unified" {
			// Add to unified servers
			if cfg.MCP.Servers == nil {
				cfg.MCP.Servers = make(map[string]config.MCPServer)
			}
			if _, exists := cfg.MCP.Servers[name]; exists {
				return fmt.Errorf("MCP server %q already exists (use 'mcp-servers remove' first)", name)
			}
			cfg.MCP.Servers[name] = server
		} else {
			// Add to backend-specific servers
			if cfg.MCP.Plugins == nil {
				cfg.MCP.Plugins = make(map[string]map[string]config.MCPServer)
			}
			if cfg.MCP.Plugins[mcpServersAddBackend] == nil {
				cfg.MCP.Plugins[mcpServersAddBackend] = make(map[string]config.MCPServer)
			}
			if _, exists := cfg.MCP.Plugins[mcpServersAddBackend][name]; exists {
				return fmt.Errorf("MCP server %q already exists for backend %s", name, mcpServersAddBackend)
			}
			cfg.MCP.Plugins[mcpServersAddBackend][name] = server
		}

		if err := cfg.Save(); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		scope := "unified (all backends)"
		if mcpServersAddBackend != "" && mcpServersAddBackend != "unified" {
			scope = mcpServersAddBackend + " only"
		}
		fmt.Printf("Added MCP server %q (%s)\n", name, scope)
		fmt.Println("Run 'scm run' or 'scm hook apply' to apply changes to backend settings.")
		return nil
	},
}

var mcpServersRemoveBackend string

var mcpServersRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"rm"},
	Short:   "Remove an MCP server configuration",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		cfg, err := GetConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		removed := false

		if mcpServersRemoveBackend == "" || mcpServersRemoveBackend == "unified" {
			if _, exists := cfg.MCP.Servers[name]; exists {
				delete(cfg.MCP.Servers, name)
				removed = true
			}
		}

		if mcpServersRemoveBackend != "" && mcpServersRemoveBackend != "unified" {
			if backendServers, ok := cfg.MCP.Plugins[mcpServersRemoveBackend]; ok {
				if _, exists := backendServers[name]; exists {
					delete(backendServers, name)
					removed = true
				}
			}
		}

		// If no specific backend, try all backends
		if mcpServersRemoveBackend == "" && !removed {
			for backend, servers := range cfg.MCP.Plugins {
				if _, exists := servers[name]; exists {
					delete(servers, name)
					removed = true
					fmt.Printf("Removed from backend: %s\n", backend)
				}
			}
		}

		if !removed {
			return fmt.Errorf("MCP server %q not found", name)
		}

		if err := cfg.Save(); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Printf("Removed MCP server %q\n", name)
		fmt.Println("Run 'scm run' or 'scm hook apply' to apply changes to backend settings.")
		return nil
	},
}

var mcpServersShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show details of an MCP server configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		cfg, err := GetConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Check unified servers
		if srv, ok := cfg.MCP.Servers[name]; ok {
			fmt.Printf("MCP Server: %s\n", name)
			fmt.Printf("Scope: unified (all backends)\n")
			fmt.Printf("Command: %s\n", srv.Command)
			if len(srv.Args) > 0 {
				fmt.Printf("Args: %s\n", strings.Join(srv.Args, " "))
			}
			if len(srv.Env) > 0 {
				fmt.Println("Environment:")
				for k, v := range srv.Env {
					fmt.Printf("  %s=%s\n", k, v)
				}
			}
			return nil
		}

		// Check backend-specific servers
		for backend, servers := range cfg.MCP.Plugins {
			if srv, ok := servers[name]; ok {
				fmt.Printf("MCP Server: %s\n", name)
				fmt.Printf("Scope: %s only\n", backend)
				fmt.Printf("Command: %s\n", srv.Command)
				if len(srv.Args) > 0 {
					fmt.Printf("Args: %s\n", strings.Join(srv.Args, " "))
				}
				if len(srv.Env) > 0 {
					fmt.Println("Environment:")
					for k, v := range srv.Env {
						fmt.Printf("  %s=%s\n", k, v)
					}
				}
				return nil
			}
		}

		return fmt.Errorf("MCP server %q not found", name)
	},
}

var mcpServersAutoRegisterDisable bool

var mcpServersAutoRegisterCmd = &cobra.Command{
	Use:   "auto-register",
	Short: "Configure auto-registration of SCM's MCP server",
	Long: `Configure whether SCM automatically registers its own MCP server.

When enabled (default), SCM injects its own MCP server into backend settings,
allowing AI agents to access SCM tools (fragments, profiles, prompts, etc.).

Examples:
  scm mcp-servers auto-register           # Show current setting
  scm mcp-servers auto-register --disable # Disable auto-registration
  scm mcp-servers auto-register --enable  # Enable auto-registration (default)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := GetConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// If flags were provided, update the setting
		if cmd.Flags().Changed("disable") || cmd.Flags().Changed("enable") {
			enabled := !mcpServersAutoRegisterDisable
			cfg.MCP.AutoRegisterSCM = &enabled

			if err := cfg.Save(); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			if enabled {
				fmt.Println("SCM MCP server auto-registration: enabled")
			} else {
				fmt.Println("SCM MCP server auto-registration: disabled")
			}
			fmt.Println("Run 'scm run' or 'scm hook apply' to apply changes to backend settings.")
			return nil
		}

		// Show current setting
		fmt.Printf("SCM MCP server auto-registration: %v\n", cfg.MCP.ShouldAutoRegisterSCM())
		return nil
	},
}

func init() {
	rootCmd.AddCommand(mcpServersCmd)

	mcpServersCmd.AddCommand(mcpServersListCmd)
	mcpServersCmd.AddCommand(mcpServersAddCmd)
	mcpServersCmd.AddCommand(mcpServersRemoveCmd)
	mcpServersCmd.AddCommand(mcpServersShowCmd)
	mcpServersCmd.AddCommand(mcpServersAutoRegisterCmd)

	mcpServersAddCmd.Flags().StringVarP(&mcpServersAddCommand, "command", "c", "", "Command to run the MCP server (required)")
	mcpServersAddCmd.Flags().StringSliceVarP(&mcpServersAddArgs, "args", "a", nil, "Arguments for the command (can be repeated)")
	mcpServersAddCmd.Flags().StringVarP(&mcpServersAddBackend, "backend", "b", "", "Backend to add server for (claude-code, gemini, or unified)")
	_ = mcpServersAddCmd.MarkFlagRequired("command")

	mcpServersRemoveCmd.Flags().StringVarP(&mcpServersRemoveBackend, "backend", "b", "", "Backend to remove server from")

	mcpServersAutoRegisterCmd.Flags().BoolVar(&mcpServersAutoRegisterDisable, "disable", false, "Disable SCM MCP server auto-registration")
	mcpServersAutoRegisterCmd.Flags().Bool("enable", false, "Enable SCM MCP server auto-registration")
}
