package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	"github.com/benjaminabbitt/scm/internal/bundles"
	"github.com/benjaminabbitt/scm/internal/collections"
	"github.com/benjaminabbitt/scm/internal/fsys"
	"github.com/benjaminabbitt/scm/internal/gitutil"
	"github.com/benjaminabbitt/scm/internal/logging"
	"github.com/benjaminabbitt/scm/internal/profiles"
	"github.com/benjaminabbitt/scm/internal/remote"
	"github.com/benjaminabbitt/scm/internal/schema"
	"github.com/benjaminabbitt/scm/resources"
)

const (
	SCMDirName          = ".scm"
	ConfigFileName      = "config"
	ContextFragmentsDir = "context-fragments"
	PromptsDir          = "prompts"
	BundlesDir          = "bundles"
)

// ConfigSource indicates where the configuration was loaded from.
type ConfigSource int

const (
	// SourceEmbedded means config was loaded from embedded resources (fallback).
	SourceEmbedded ConfigSource = iota
	// SourceProject means config was loaded from a project .scm directory.
	SourceProject
)

// Config holds the SCM configuration.
type Config struct {
	LM         LMConfig             `mapstructure:"lm"`
	Editor     EditorConfig         `mapstructure:"editor"`
	Defaults   Defaults             `mapstructure:"defaults"`
	Hooks      HooksConfig          `mapstructure:"hooks"`
	MCP        MCPConfig            `mapstructure:"mcp"`
	Profiles   map[string]Profile   `mapstructure:"profiles"`
	Generators map[string]Generator `mapstructure:"generators"`
	SCMPaths   []string             // Resolved .scm directory (at most one)
	Source     ConfigSource         // Where the configuration was loaded from
}

// EditorConfig holds editor-related configuration.
type EditorConfig struct {
	Command string   `mapstructure:"command" yaml:"command,omitempty"` // Editor command (default: nano)
	Args    []string `mapstructure:"args" yaml:"args,omitempty"`       // Additional arguments
}

// GetEditorCommand returns the editor command to use.
// Checks in order: config, VISUAL env, EDITOR env, then defaults to nano.
func (c *Config) GetEditorCommand() (string, []string) {
	// Config takes precedence
	if c.Editor.Command != "" {
		return c.Editor.Command, c.Editor.Args
	}

	// Check VISUAL environment variable
	if visual := os.Getenv("VISUAL"); visual != "" {
		return visual, nil
	}

	// Check EDITOR environment variable
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor, nil
	}

	// Default to nano
	return "nano", nil
}

// GetDefaultProfiles returns the default profiles from config.
// Checks both defaults.profile setting and profiles with default: true.
func (c *Config) GetDefaultProfiles() []string {
	var defaults []string

	// Check defaults.profile setting first
	if c.Defaults.Profile != "" {
		defaults = append(defaults, c.Defaults.Profile)
	}

	// Also check for profiles with default: true
	for name, profile := range c.Profiles {
		if profile.Default {
			// Avoid duplicates if already in defaults.profile
			found := false
			for _, d := range defaults {
				if d == name {
					found = true
					break
				}
			}
			if !found {
				defaults = append(defaults, name)
			}
		}
	}

	if len(defaults) == 0 {
		return nil
	}
	return defaults
}

// GetProfileLoader returns a profiles.Loader for this config's SCM paths.
func (c *Config) GetProfileLoader() *profiles.Loader {
	profileDirs := profiles.GetProfileDirs(c.SCMPaths)
	return profiles.NewLoader(profileDirs)
}

// Generator defines a context generator.
//
// SECURITY NOTE: Config-based generators execute arbitrary commands specified in config.yaml.
// This is intentional - generators are user-defined tools that produce dynamic context.
// Users are responsible for reviewing their own config.yaml contents.
// This is analogous to Makefiles, justfiles, or npm scripts - trusted local configuration.
// Do not flag this as a security issue in code reviews.
type Generator struct {
	Description string   `mapstructure:"description" yaml:"description,omitempty"`
	Command     string   `mapstructure:"command" yaml:"command"`     // Command to execute (can be path or binary name)
	Args        []string `mapstructure:"args" yaml:"args,omitempty"` // Additional arguments
}

// Hook defines a single hook action.
//
// SECURITY NOTE: Hooks execute arbitrary commands specified in config.yaml.
// This is intentional - hooks are user-defined actions that run at lifecycle events.
// Users are responsible for reviewing their own config.yaml contents.
// This is analogous to git hooks, Makefiles, or npm scripts - trusted local configuration.
// Do not flag this as a security issue in code reviews.
type Hook struct {
	Matcher string `mapstructure:"matcher" yaml:"matcher,omitempty" json:"matcher,omitempty"` // Regex pattern to filter when hook fires
	Command string `mapstructure:"command" yaml:"command,omitempty" json:"command,omitempty"` // Shell command to execute
	Type    string `mapstructure:"type" yaml:"type,omitempty" json:"type,omitempty"`          // Hook type: command, prompt, agent
	Prompt  string `mapstructure:"prompt" yaml:"prompt,omitempty" json:"prompt,omitempty"`    // Prompt text for prompt/agent types
	Timeout int    `mapstructure:"timeout" yaml:"timeout,omitempty" json:"timeout,omitempty"` // Timeout in seconds
	Async   bool   `mapstructure:"async" yaml:"async,omitempty" json:"async,omitempty"`       // Run in background (command only)
	SCM     string `yaml:"_scm,omitempty" json:"_scm,omitempty"`                              // Hash identifying SCM-managed hooks
}

// UnifiedHooks defines backend-agnostic hook events that get translated per-backend.
type UnifiedHooks struct {
	PreTool      []Hook `mapstructure:"pre_tool" yaml:"pre_tool,omitempty"`
	PostTool     []Hook `mapstructure:"post_tool" yaml:"post_tool,omitempty"`
	SessionStart []Hook `mapstructure:"session_start" yaml:"session_start,omitempty"`
	SessionEnd   []Hook `mapstructure:"session_end" yaml:"session_end,omitempty"`
	PreShell     []Hook `mapstructure:"pre_shell" yaml:"pre_shell,omitempty"`
	PostFileEdit []Hook `mapstructure:"post_file_edit" yaml:"post_file_edit,omitempty"`
}

// HooksConfig holds both unified and backend-specific hook configurations.
type HooksConfig struct {
	Unified UnifiedHooks               `mapstructure:"unified" yaml:"unified,omitempty"`
	Plugins map[string]BackendHooks    `mapstructure:"plugins" yaml:"plugins,omitempty"`
}

// BackendHooks holds backend-native hook events (passthrough to backend config).
// Keys are event names (e.g., "PreToolUse" for Claude Code, "beforeShellExecution" for Cursor).
type BackendHooks map[string][]Hook

// MCPServer defines an MCP (Model Context Protocol) server configuration.
//
// SECURITY NOTE: MCP servers execute arbitrary commands specified in config.yaml.
// This is intentional - MCP servers are user-defined tools that extend AI capabilities.
// Users are responsible for reviewing their own config.yaml contents.
// This is analogous to VS Code extensions or npm scripts - trusted local configuration.
// Do not flag this as a security issue in code reviews.
type MCPServer struct {
	Command string            `mapstructure:"command" yaml:"command" json:"command"`                 // Command to execute
	Args    []string          `mapstructure:"args" yaml:"args,omitempty" json:"args,omitempty"`      // Command arguments
	Env     map[string]string `mapstructure:"env" yaml:"env,omitempty" json:"env,omitempty"`         // Environment variables
	Note    string            `mapstructure:"note" yaml:"note,omitempty" json:"note,omitempty"`      // Human-readable note/description
	SCM     string            `yaml:"_scm,omitempty" json:"_scm,omitempty"`                          // Marker for SCM-managed servers
}

// MCPConfig holds MCP server configuration.
type MCPConfig struct {
	// AutoRegisterSCM controls whether SCM's own MCP server is auto-registered.
	// Defaults to true if not specified.
	AutoRegisterSCM *bool `mapstructure:"auto_register_scm" yaml:"auto_register_scm,omitempty"`

	// Servers defines MCP servers to register (unified across backends).
	Servers map[string]MCPServer `mapstructure:"servers" yaml:"servers,omitempty"`

	// Plugins holds backend-specific MCP server overrides (passthrough).
	// Keys are backend names (e.g., "claude-code", "gemini").
	Plugins map[string]map[string]MCPServer `mapstructure:"plugins" yaml:"plugins,omitempty"`
}

// ShouldAutoRegisterSCM returns whether to auto-register the SCM MCP server.
// Defaults to true if not explicitly set.
func (m *MCPConfig) ShouldAutoRegisterSCM() bool {
	if m == nil || m.AutoRegisterSCM == nil {
		return true
	}
	return *m.AutoRegisterSCM
}

// PluginConfig holds configuration for a specific AI plugin.
type PluginConfig struct {
	Default    bool              `mapstructure:"default" yaml:"default,omitempty"` // If true, this is the default plugin
	Model      string            `mapstructure:"model" yaml:"model,omitempty"`     // Default model for this plugin
	BinaryPath string            `mapstructure:"binary_path" yaml:"binary_path,omitempty"`
	Args       []string          `mapstructure:"args" yaml:"args,omitempty"`
	Env        map[string]string `mapstructure:"env" yaml:"env,omitempty"`
}

// LMConfig holds LM (language model) configuration.
type LMConfig struct {
	PluginPaths []string                `mapstructure:"plugin_paths" yaml:"plugin_paths,omitempty"`
	Plugins     map[string]PluginConfig `mapstructure:"plugins" yaml:"plugins"`
}

// GetDefaultPlugin returns the name of the default plugin.
// Returns the first plugin marked as default, or "claude-code" as fallback.
func (c *LMConfig) GetDefaultPlugin() string {
	for name, cfg := range c.Plugins {
		if cfg.Default {
			return name
		}
	}
	return "claude-code"
}

// SetDefaultPlugin sets the named plugin as the default, clearing all others.
// If the plugin doesn't exist in the map, it creates a new entry.
func (c *LMConfig) SetDefaultPlugin(name string) {
	if c.Plugins == nil {
		c.Plugins = make(map[string]PluginConfig)
	}
	for k, cfg := range c.Plugins {
		cfg.Default = false
		c.Plugins[k] = cfg
	}
	cfg := c.Plugins[name]
	cfg.Default = true
	c.Plugins[name] = cfg
}

// GetDefaultModel returns the default model for the specified plugin.
// Returns empty string if no default is configured.
func (c *LMConfig) GetDefaultModel(pluginName string) string {
	if cfg, ok := c.Plugins[pluginName]; ok {
		return cfg.Model
	}
	return ""
}

// Profile is a named collection of context fragments, variables, and context generators.
// Fragments can be specified directly by path, or dynamically via tags.
// Profiles can inherit from parent profiles using the Parents field.
type Profile struct {
	Default     bool              `mapstructure:"default" yaml:"default,omitempty"`       // Whether this is a default profile
	Description string            `mapstructure:"description" yaml:"description,omitempty"`
	Parents     []string          `mapstructure:"parents" yaml:"parents,omitempty"`       // Parent profiles to inherit from
	Tags        []string          `mapstructure:"tags" yaml:"tags,omitempty"`             // Fragment tags to include
	Bundles     []string          `mapstructure:"bundles" yaml:"bundles,omitempty"`       // Bundle references (e.g., "remote/go-tools")
	BundleItems []string          `mapstructure:"bundle_items" yaml:"bundle_items,omitempty"` // Cherry-pick items (e.g., "remote/bundle:fragments/name")
	Fragments   []string          `mapstructure:"fragments" yaml:"fragments,omitempty"`   // Explicit fragment paths (local/legacy)
	Variables   map[string]string `mapstructure:"variables" yaml:"variables,omitempty"`
	Generators  []string          `mapstructure:"generators" yaml:"generators,omitempty"` // Plugin binaries that output context
	Hooks       HooksConfig       `mapstructure:"hooks" yaml:"hooks,omitempty"`           // Hooks for this profile (inherited)
	MCP         MCPConfig         `mapstructure:"mcp" yaml:"mcp,omitempty"`               // MCP servers for this profile (inherited)
	MCPServers  []string          `mapstructure:"mcp_servers" yaml:"mcp_servers,omitempty"` // Remote MCP server references (legacy)
}

// Defaults holds default settings applied when no explicit values are specified.
type Defaults struct {
	Profile      string   `mapstructure:"profile" yaml:"profile,omitempty"`             // Default profile to load
	Generators   []string `mapstructure:"generators" yaml:"generators,omitempty"`       // Generators always run
	UseDistilled *bool    `mapstructure:"use_distilled" yaml:"use_distilled,omitempty"` // Prefer .distilled.md versions (default true)
}

// ShouldUseDistilled returns whether to prefer distilled versions of fragments/prompts.
// Defaults to true if not explicitly set.
func (d *Defaults) ShouldUseDistilled() bool {
	if d.UseDistilled == nil {
		return true
	}
	return *d.UseDistilled
}

// Load finds and loads configuration from a single source.
// Priority order (first found wins, no merging):
//  1. Project .scm directory (at git root)
//  2. Embedded resources (fallback)
func Load() (*Config, error) {
	cfg := &Config{
		LM: LMConfig{
			Plugins: make(map[string]PluginConfig),
		},
		Profiles:   make(map[string]Profile),
		Generators: make(map[string]Generator),
	}

	// Create config validator for schema validation
	configValidator, err := schema.NewConfigValidator()
	if err != nil {
		logging.L().Warn("failed to create config validator",
			logging.ErrorField(err))
		configValidator = nil
	}

	// Try project .scm directory first
	scmPath, source := findSCMDir()
	if scmPath != "" {
		cfg.SCMPaths = []string{scmPath}
		cfg.Source = source

		configPath := filepath.Join(scmPath, ConfigFileName+".yaml")
		if err := loadConfigFile(cfg, configPath, configValidator); err != nil {
			return nil, err
		}
		return cfg, nil
	}

	// Fall back to embedded resources
	cfg.Source = SourceEmbedded
	embeddedCfg, err := LoadEmbeddedConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load embedded config: %w", err)
	}

	// Merge embedded config into cfg with nil checks
	cfg.LM = embeddedCfg.LM
	if cfg.LM.Plugins == nil {
		cfg.LM.Plugins = make(map[string]PluginConfig)
	}
	cfg.Defaults = embeddedCfg.Defaults
	if embeddedCfg.Profiles != nil {
		cfg.Profiles = embeddedCfg.Profiles
	}
	if embeddedCfg.Generators != nil {
		cfg.Generators = embeddedCfg.Generators
	}

	logging.L().Debug("using embedded configuration")
	return cfg, nil
}

// loadConfigFile loads a config file into the provided Config struct.
func loadConfigFile(cfg *Config, configPath string, validator *schema.ConfigValidator) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Config file is optional
			return nil
		}
		return fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	// Validate against schema before parsing
	if validator != nil {
		if err := validator.ValidateBytes(data); err != nil {
			return fmt.Errorf("config validation failed at %s: %w", configPath, err)
		}
	}

	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return nil
		}
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read config at %s: %w", configPath, err)
	}

	if err := v.Unmarshal(cfg); err != nil {
		return fmt.Errorf("failed to parse config at %s: %w", configPath, err)
	}

	logging.L().Debug(logging.MsgConfigLoaded, logging.FilePath(configPath))
	return nil
}

// findSCMDir locates the .scm directory at the git root.
// Returns the path and source, or empty string if not found.
func findSCMDir() (string, ConfigSource) {
	// Try to find git root
	pwd, err := os.Getwd()
	if err != nil {
		logging.L().Warn("failed to get working directory", logging.ErrorField(err))
		return "", SourceEmbedded
	}

	// Check for project .scm at git root
	gitRoot, err := gitutil.FindRoot(pwd)
	if err == nil {
		scmPath := filepath.Join(gitRoot, SCMDirName)
		if info, err := os.Stat(scmPath); err == nil && info.IsDir() {
			return scmPath, SourceProject
		}
	}

	// No project .scm found, fall back to embedded resources
	return "", SourceEmbedded
}

// GetFragmentDirs returns context-fragments directories.
// For embedded source, returns ["."] (use with GetFragmentFS).
func (c *Config) GetFragmentDirs() []string {
	if c.Source == SourceEmbedded {
		return []string{"."}
	}
	var dirs []string
	for _, scmPath := range c.SCMPaths {
		fragDir := filepath.Join(scmPath, ContextFragmentsDir)
		if info, err := os.Stat(fragDir); err == nil && info.IsDir() {
			dirs = append(dirs, fragDir)
		}
	}
	return dirs
}

// GetPromptDirs returns prompts directories.
// For embedded source, returns ["."] (use with GetPromptFS).
func (c *Config) GetPromptDirs() []string {
	if c.Source == SourceEmbedded {
		return []string{"."}
	}
	var dirs []string
	for _, scmPath := range c.SCMPaths {
		promptDir := filepath.Join(scmPath, PromptsDir)
		if info, err := os.Stat(promptDir); err == nil && info.IsDir() {
			dirs = append(dirs, promptDir)
		}
	}
	return dirs
}

// GetBundleDirs returns bundles directories.
// Bundles are not embedded, so returns empty for embedded source.
func (c *Config) GetBundleDirs() []string {
	if c.Source == SourceEmbedded {
		return nil
	}
	var dirs []string
	for _, scmPath := range c.SCMPaths {
		bundleDir := filepath.Join(scmPath, BundlesDir)
		if info, err := os.Stat(bundleDir); err == nil && info.IsDir() {
			dirs = append(dirs, bundleDir)
		}
	}
	return dirs
}

// IsEmbedded returns true if using embedded resources (no .scm directory found).
func (c *Config) IsEmbedded() bool {
	return c.Source == SourceEmbedded
}

// SourceName returns a human-readable name for the config source.
func (c *Config) SourceName() string {
	switch c.Source {
	case SourceProject:
		return "project"
	case SourceEmbedded:
		return "embedded"
	default:
		return "unknown"
	}
}

// GetFragmentFS returns an fsys.FS for loading fragments.
// For embedded source, returns an EmbedFS wrapper.
// For project/home sources, returns nil (use GetFragmentDirs with OS filesystem).
func (c *Config) GetFragmentFS() fsys.FS {
	if c.Source == SourceEmbedded {
		return fsys.NewEmbedFS(resources.FragmentsFS(), ContextFragmentsDir)
	}
	return nil
}

// GetPromptFS returns an fsys.FS for loading prompts.
// For embedded source, returns an EmbedFS wrapper.
// For project/home sources, returns nil (use GetPromptDirs with OS filesystem).
func (c *Config) GetPromptFS() fsys.FS {
	if c.Source == SourceEmbedded {
		return fsys.NewEmbedFS(resources.PromptsFS(), PromptsDir)
	}
	return nil
}

// GetPluginPaths returns the paths where external plugins are searched for.
// Defaults to .scm/plugins if not configured.
func (c *Config) GetPluginPaths() []string {
	if len(c.LM.PluginPaths) > 0 {
		return c.LM.PluginPaths
	}
	// Default plugin paths from project .scm
	var paths []string
	for _, scmPath := range c.SCMPaths {
		paths = append(paths, filepath.Join(scmPath, "plugins"))
	}
	return paths
}

// GetGeneratorPaths returns the paths where external generators are searched for.
// Defaults to .scm/generators.
func (c *Config) GetGeneratorPaths() []string {
	// Default generator paths from project .scm
	var paths []string
	for _, scmPath := range c.SCMPaths {
		paths = append(paths, filepath.Join(scmPath, "generators"))
	}
	return paths
}

// ConfigFile represents the structure for saving config.yaml
type ConfigFile struct {
	LM         LMConfig             `yaml:"lm"`
	Editor     EditorConfig         `yaml:"editor,omitempty"`
	Defaults   Defaults             `yaml:"defaults,omitempty"`
	Hooks      HooksConfig          `yaml:"hooks,omitempty"`
	Profiles   map[string]Profile   `yaml:"profiles,omitempty"`
	Generators map[string]Generator `yaml:"generators,omitempty"`
}

// GetConfigFilePath returns the path to the primary config file.
// Uses the closest project .scm directory.
func (c *Config) GetConfigFilePath() (string, error) {
	if len(c.SCMPaths) == 0 {
		return "", fmt.Errorf("no .scm directory found; run 'scm init --local' first")
	}
	return filepath.Join(c.SCMPaths[0], ConfigFileName+".yaml"), nil
}

// Save writes the configuration to the primary config file.
func (c *Config) Save() error {
	configPath, err := c.GetConfigFilePath()
	if err != nil {
		return fmt.Errorf("failed to get config path: %w", err)
	}

	// Read existing config to preserve unknown fields
	existingData, _ := os.ReadFile(configPath)
	existing := make(map[string]interface{})
	if len(existingData) > 0 {
		yaml.Unmarshal(existingData, &existing)
	}

	// Update with current values
	existing["lm"] = c.LM
	if len(c.Defaults.Generators) > 0 {
		existing["defaults"] = c.Defaults
	}
	if len(c.Profiles) > 0 {
		existing["profiles"] = c.Profiles
	}
	if len(c.Generators) > 0 {
		existing["generators"] = c.Generators
	}

	data, err := yaml.Marshal(existing)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// LoadEmbedded loads the embedded default configuration, returning nil on error.
// Use this when you want to check embedded resources without failing.
func LoadEmbedded() (*Config, error) {
	return LoadEmbeddedConfig()
}

// LoadEmbeddedConfig loads the embedded default configuration.
func LoadEmbeddedConfig() (*Config, error) {
	data, err := resources.GetEmbeddedConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded config: %w", err)
	}

	cfg := &Config{
		Profiles:   make(map[string]Profile),
		Generators: make(map[string]Generator),
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse embedded config: %w", err)
	}

	return cfg, nil
}

// LoadFromDir loads config from a specific .scm directory.
func LoadFromDir(scmDir string) (*Config, error) {
	cfg := &Config{
		Profiles:   make(map[string]Profile),
		Generators: make(map[string]Generator),
	}

	configPath := filepath.Join(scmDir, ConfigFileName+".yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty config if no config file exists
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config from %s: %w", configPath, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config from %s: %w", configPath, err)
	}

	cfg.SCMPaths = []string{scmDir}
	return cfg, nil
}

// MergeProfiles merges profiles from source into target.
// Source profiles override target profiles with the same name.
func MergeProfiles(target, source map[string]Profile) {
	for name, profile := range source {
		target[name] = profile
	}
}

// MergeGenerators merges generators from source into target.
// Source generators override target generators with the same name.
func MergeGenerators(target, source map[string]Generator) {
	for name, gen := range source {
		target[name] = gen
	}
}

// CollectFragmentsForProfiles returns a deduplicated list of all fragments
// referenced by the specified profiles.
func CollectFragmentsForProfiles(profiles map[string]Profile, profileNames []string) ([]string, error) {
	seen := collections.NewSet[string]()
	var fragments []string

	for _, name := range profileNames {
		profile, ok := profiles[name]
		if !ok {
			return nil, fmt.Errorf("unknown profile: %s", name)
		}
		for _, frag := range profile.Fragments {
			if !seen.Has(frag) {
				seen.Add(frag)
				fragments = append(fragments, frag)
			}
		}
	}

	return fragments, nil
}

// CollectGeneratorsForProfiles returns a deduplicated list of all generators
// referenced by the specified profiles.
func CollectGeneratorsForProfiles(profiles map[string]Profile, profileNames []string) []string {
	seen := collections.NewSet[string]()
	var generators []string

	for _, name := range profileNames {
		profile, ok := profiles[name]
		if !ok {
			continue
		}
		for _, gen := range profile.Generators {
			if !seen.Has(gen) {
				seen.Add(gen)
				generators = append(generators, gen)
			}
		}
	}

	return generators
}

// CollectBundlesForProfiles returns a deduplicated list of all bundles
// referenced by the specified profiles.
func CollectBundlesForProfiles(profiles map[string]Profile, profileNames []string) ([]string, error) {
	seen := collections.NewSet[string]()
	var bundles []string

	for _, name := range profileNames {
		profile, ok := profiles[name]
		if !ok {
			return nil, fmt.Errorf("unknown profile: %s", name)
		}
		for _, bundle := range profile.Bundles {
			if !seen.Has(bundle) {
				seen.Add(bundle)
				bundles = append(bundles, bundle)
			}
		}
	}

	return bundles, nil
}

// CollectBundleItemsForProfiles returns a deduplicated list of all cherry-picked
// bundle items referenced by the specified profiles.
func CollectBundleItemsForProfiles(profiles map[string]Profile, profileNames []string) ([]string, error) {
	seen := collections.NewSet[string]()
	var items []string

	for _, name := range profileNames {
		profile, ok := profiles[name]
		if !ok {
			return nil, fmt.Errorf("unknown profile: %s", name)
		}
		for _, item := range profile.BundleItems {
			if !seen.Has(item) {
				seen.Add(item)
				items = append(items, item)
			}
		}
	}

	return items, nil
}

// FilterProfiles returns only the specified profiles from the full map.
func FilterProfiles(all map[string]Profile, names []string) map[string]Profile {
	filtered := make(map[string]Profile)
	for _, name := range names {
		if profile, ok := all[name]; ok {
			filtered[name] = profile
		}
	}
	return filtered
}

// FilterGenerators returns only the specified generators from the full map.
func FilterGenerators(all map[string]Generator, names []string) map[string]Generator {
	filtered := make(map[string]Generator)
	for _, name := range names {
		if gen, ok := all[name]; ok {
			filtered[name] = gen
		}
	}
	return filtered
}

// profileBuilder collects profile fields using sets to avoid duplicates during inheritance.
type profileBuilder struct {
	Description string
	Tags        collections.Set[string]
	Bundles     collections.Set[string]
	BundleItems collections.Set[string]
	Fragments   collections.Set[string]
	Generators  collections.Set[string]
	Variables   map[string]string
	Hooks       HooksConfig
	MCP         MCPConfig
	// Track insertion order for stable output
	tagsOrder        []string
	bundlesOrder     []string
	bundleItemsOrder []string
	fragmentsOrder   []string
	generatorsOrder  []string
	// Track seen hooks by key (command+matcher) for deduplication
	seenHooks collections.Set[string]
}

func newProfileBuilder() *profileBuilder {
	return &profileBuilder{
		Tags:        collections.NewSet[string](),
		Bundles:     collections.NewSet[string](),
		BundleItems: collections.NewSet[string](),
		Fragments:   collections.NewSet[string](),
		Generators:  collections.NewSet[string](),
		Variables:   make(map[string]string),
		Hooks: HooksConfig{
			Plugins: make(map[string]BackendHooks),
		},
		MCP: MCPConfig{
			Servers: make(map[string]MCPServer),
			Plugins: make(map[string]map[string]MCPServer),
		},
		seenHooks: collections.NewSet[string](),
	}
}

func (b *profileBuilder) addTag(tag string) {
	if !b.Tags.Has(tag) {
		b.Tags.Add(tag)
		b.tagsOrder = append(b.tagsOrder, tag)
	}
}

func (b *profileBuilder) addBundle(bundle string) {
	if !b.Bundles.Has(bundle) {
		b.Bundles.Add(bundle)
		b.bundlesOrder = append(b.bundlesOrder, bundle)
	}
}

func (b *profileBuilder) addBundleItem(item string) {
	if !b.BundleItems.Has(item) {
		b.BundleItems.Add(item)
		b.bundleItemsOrder = append(b.bundleItemsOrder, item)
	}
}

func (b *profileBuilder) addFragment(frag string) {
	if !b.Fragments.Has(frag) {
		b.Fragments.Add(frag)
		b.fragmentsOrder = append(b.fragmentsOrder, frag)
	}
}

func (b *profileBuilder) addGenerator(gen string) {
	if !b.Generators.Has(gen) {
		b.Generators.Add(gen)
		b.generatorsOrder = append(b.generatorsOrder, gen)
	}
}

// hookKey returns a unique key for deduplication based on command and matcher.
func hookKey(h Hook) string {
	return h.Command + "|" + h.Matcher
}

// addHook adds a hook if not already present (by command+matcher key).
func (b *profileBuilder) addHook(hooks *[]Hook, h Hook) {
	key := hookKey(h)
	if !b.seenHooks.Has(key) {
		b.seenHooks.Add(key)
		*hooks = append(*hooks, h)
	}
}

// mergeMCP merges MCP config from source into the builder.
// Later sources override earlier ones for the same server name.
func (b *profileBuilder) mergeMCP(source MCPConfig) {
	// Merge auto_register_scm (later wins)
	if source.AutoRegisterSCM != nil {
		b.MCP.AutoRegisterSCM = source.AutoRegisterSCM
	}

	// Merge unified servers
	for name, server := range source.Servers {
		b.MCP.Servers[name] = server
	}

	// Merge plugin-specific servers
	for backend, servers := range source.Plugins {
		if b.MCP.Plugins[backend] == nil {
			b.MCP.Plugins[backend] = make(map[string]MCPServer)
		}
		for name, server := range servers {
			b.MCP.Plugins[backend][name] = server
		}
	}
}

// mergeHooks merges hooks from source into the builder.
func (b *profileBuilder) mergeHooks(source HooksConfig) {
	// Merge unified hooks
	for _, h := range source.Unified.PreTool {
		b.addHook(&b.Hooks.Unified.PreTool, h)
	}
	for _, h := range source.Unified.PostTool {
		b.addHook(&b.Hooks.Unified.PostTool, h)
	}
	for _, h := range source.Unified.SessionStart {
		b.addHook(&b.Hooks.Unified.SessionStart, h)
	}
	for _, h := range source.Unified.SessionEnd {
		b.addHook(&b.Hooks.Unified.SessionEnd, h)
	}
	for _, h := range source.Unified.PreShell {
		b.addHook(&b.Hooks.Unified.PreShell, h)
	}
	for _, h := range source.Unified.PostFileEdit {
		b.addHook(&b.Hooks.Unified.PostFileEdit, h)
	}

	// Merge plugin-specific hooks
	for pluginName, backendHooks := range source.Plugins {
		if b.Hooks.Plugins[pluginName] == nil {
			b.Hooks.Plugins[pluginName] = make(BackendHooks)
		}
		for eventName, hooks := range backendHooks {
			for _, h := range hooks {
				key := pluginName + ":" + eventName + ":" + hookKey(h)
				if !b.seenHooks.Has(key) {
					b.seenHooks.Add(key)
					b.Hooks.Plugins[pluginName][eventName] = append(b.Hooks.Plugins[pluginName][eventName], h)
				}
			}
		}
	}
}

func (b *profileBuilder) toProfile() *Profile {
	return &Profile{
		Description: b.Description,
		Tags:        b.tagsOrder,
		Bundles:     b.bundlesOrder,
		BundleItems: b.bundleItemsOrder,
		Fragments:   b.fragmentsOrder,
		Generators:  b.generatorsOrder,
		Variables:   b.Variables,
		Hooks:       b.Hooks,
		MCP:         b.MCP,
	}
}

// maxProfileDepth is the maximum allowed depth for profile inheritance.
// This prevents stack overflow from deeply nested or malformed configurations.
// The value 64 is arbitrary but well beyond any reasonable inheritance chain.
const maxProfileDepth = 64

// ResolveProfile resolves a profile by recursively merging all parent profiles.
// Parents are processed depth-first, with later parents and the child overriding earlier values.
// Uses sets internally to handle diamond inheritance (shared ancestors) without duplicates.
// Returns an error if the profile doesn't exist or if circular dependencies are detected.
func ResolveProfile(profiles map[string]Profile, name string) (*Profile, error) {
	visited := collections.NewSet[string]()
	builder := newProfileBuilder()
	if err := resolveProfileRecursive(profiles, name, visited, builder, 0); err != nil {
		return nil, err
	}
	return builder.toProfile(), nil
}

func resolveProfileRecursive(profiles map[string]Profile, name string, visited collections.Set[string], builder *profileBuilder, depth int) error {
	// Check depth limit
	if depth > maxProfileDepth {
		return fmt.Errorf("profile inheritance depth exceeds maximum (%d): possible misconfiguration", maxProfileDepth)
	}

	// Check for circular dependency
	if visited.Has(name) {
		return fmt.Errorf("circular profile inheritance detected: %s", name)
	}
	visited.Add(name)

	profile, ok := profiles[name]
	if !ok {
		return fmt.Errorf("unknown profile: %s", name)
	}

	// Resolve parents first (depth-first)
	for _, parentName := range profile.Parents {
		if err := resolveProfileRecursive(profiles, parentName, visited.Clone(), builder, depth+1); err != nil {
			return fmt.Errorf("failed to resolve parent %s: %w", parentName, err)
		}
	}

	// Merge this profile's values (child overrides parents for variables)
	for _, tag := range profile.Tags {
		builder.addTag(tag)
	}
	for _, bundle := range profile.Bundles {
		builder.addBundle(bundle)
	}
	for _, item := range profile.BundleItems {
		builder.addBundleItem(item)
	}
	for _, frag := range profile.Fragments {
		builder.addFragment(frag)
	}
	for _, gen := range profile.Generators {
		builder.addGenerator(gen)
	}
	for k, v := range profile.Variables {
		builder.Variables[k] = v
	}

	// Merge hooks (deduplicated by command+matcher)
	builder.mergeHooks(profile.Hooks)

	// Merge MCP config (later wins for same server names)
	builder.mergeMCP(profile.MCP)

	// Set description from the leaf profile (will be overwritten by each child)
	builder.Description = profile.Description

	return nil
}

// DedupeStrings removes duplicates from a string slice while preserving order.
func DedupeStrings(items []string) []string {
	seen := collections.NewSet[string]()
	result := make([]string, 0, len(items))
	for _, item := range items {
		if !seen.Has(item) {
			seen.Add(item)
			result = append(result, item)
		}
	}
	return result
}

// ResolveBundleMCPServers loads MCP servers from bundles referenced in the default profile.
// It returns a map of server name to MCPServer configuration.
func (c *Config) ResolveBundleMCPServers() map[string]MCPServer {
	result := make(map[string]MCPServer)

	// Get the default profile name
	defaultProfile := c.Defaults.Profile
	if defaultProfile == "" {
		return result
	}

	// Get the base .scm directory
	if len(c.SCMPaths) == 0 {
		return result
	}
	scmDir := c.SCMPaths[0]

	// Load the profile using profiles package
	profileLoader := c.GetProfileLoader()
	profile, err := profileLoader.Load(defaultProfile)
	if err != nil {
		return result
	}

	// Create bundle loader
	bundleDirs := []string{filepath.Join(scmDir, BundlesDir)}
	bundleLoader := bundles.NewLoader(bundleDirs, false)

	// Process each bundle URL in the profile
	for _, bundleRef := range profile.Bundles {
		servers := loadMCPFromBundleRef(bundleRef, scmDir, bundleLoader)
		for name, server := range servers {
			result[name] = server
		}
	}

	return result
}

// loadMCPFromBundleRef loads MCP servers from a bundle reference (URL or name).
func loadMCPFromBundleRef(bundleRef string, scmDir string, loader *bundles.Loader) map[string]MCPServer {
	result := make(map[string]MCPServer)

	// Parse the reference to get the local path
	ref, err := remote.ParseReference(bundleRef)
	if err != nil {
		// Try as a local bundle name
		bundle, err := loader.Load(bundleRef)
		if err != nil {
			return result
		}
		return extractMCPFromBundle(bundle, bundleRef)
	}

	// Get the local path for this bundle
	localPath := ref.LocalPath(scmDir, remote.ItemTypeBundle)

	// Load the bundle from the local path
	bundle, err := loader.LoadFile(localPath)
	if err != nil {
		return result
	}

	return extractMCPFromBundle(bundle, bundleRef)
}

// extractMCPFromBundle extracts MCP servers from a loaded bundle.
func extractMCPFromBundle(bundle *bundles.Bundle, source string) map[string]MCPServer {
	result := make(map[string]MCPServer)

	for name, mcp := range bundle.MCP {
		result[name] = MCPServer{
			Command: mcp.Command,
			Args:    mcp.Args,
			Env:     mcp.Env,
			Note:    mcp.Note,
			SCM:     "bundle:" + source, // Mark as coming from a bundle
		}
	}

	return result
}
