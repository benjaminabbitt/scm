package backends

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/benjaminabbitt/scm/internal/config"
	"github.com/benjaminabbitt/scm/internal/fragments"
	pb "github.com/benjaminabbitt/scm/internal/lm/grpc"
	"github.com/benjaminabbitt/scm/internal/ptyrunner"
)

// ClaudeCode implements the Backend interface for Claude Code CLI.
type ClaudeCode struct {
	BaseBackend
	BinaryPath string
	Args       []string
	Env        map[string]string
}

// NewClaudeCode creates a new Claude Code backend with default settings.
func NewClaudeCode() *ClaudeCode {
	return &ClaudeCode{
		BaseBackend: NewBaseBackend("claude-code", "1.0.0"),
		BinaryPath:  "claude",
		Args:        []string{},
		Env:         make(map[string]string),
	}
}

// Configure applies plugin configuration to this backend.
func (b *ClaudeCode) Configure(cfg *config.PluginConfig) {
	if cfg.BinaryPath != "" {
		b.BinaryPath = cfg.BinaryPath
	}
	if len(cfg.Args) > 0 {
		b.Args = cfg.Args
	}
	for k, v := range cfg.Env {
		b.Env[k] = v
	}
}

// Run executes Claude Code with the given request.
func (b *ClaudeCode) Run(ctx context.Context, req *pb.RunRequest, stdout, stderr io.Writer) (int32, *pb.ModelInfo, error) {
	opts := req.GetOptions()
	if opts == nil {
		opts = &pb.RunOptions{}
	}

	// Determine work directory
	workDir := opts.WorkDir
	if workDir == "" {
		workDir = "."
	}

	// Write session-scoped context file
	session, err := WriteSessionContext(workDir, req.Fragments)
	if err != nil {
		fmt.Fprintf(stderr, "warning: failed to write session context: %v\n", err)
	}
	// Ensure cleanup on exit
	if session != nil && session.ID != "" {
		defer CleanupSessionContext(workDir, session.ID)
	}

	// Write Claude Code slash command files from prompts
	if prompts := b.loadPrompts(workDir, stderr); len(prompts) > 0 {
		if err := WriteCommandFiles(workDir, prompts); err != nil {
			fmt.Fprintf(stderr, "warning: failed to write command files: %v\n", err)
		}
	}

	// Write hooks to .claude/settings.json (includes auto-registered SessionStart hook)
	if err := b.writeHooks(workDir, stderr); err != nil {
		fmt.Fprintf(stderr, "warning: failed to write hooks: %v\n", err)
	}

	// Add session ID to environment for hook injection
	if session != nil && session.ID != "" {
		if opts.Env == nil {
			opts.Env = make(map[string]string)
		}
		opts.Env[SCMContextIDEnv] = session.ID
	}

	args := b.buildArgs(req)

	// Verbosity level 16+: show command
	if opts.Verbosity >= 16 {
		fmt.Fprintf(stderr, "[v16] %s %s\n", b.BinaryPath, strings.Join(args, " "))
	}

	// Build model info - priority: request opts > config default > hardcoded fallback
	modelName := opts.Model
	if modelName == "" {
		if cfg, err := config.Load(); err == nil {
			modelName = cfg.LM.GetDefaultModel(b.Name())
		}
	}
	if modelName == "" {
		modelName = "claude-3-opus" // fallback
	}
	modelInfo := &pb.ModelInfo{
		ModelName: modelName,
		Provider:  "anthropic",
	}

	// Dry run - return without executing
	if opts.DryRun {
		return 0, modelInfo, nil
	}

	// Use PTY for interactive mode
	if opts.Mode == pb.ExecutionMode_INTERACTIVE {
		exitCode, err := b.runInteractive(ctx, req, args, stdout, stderr)
		return exitCode, modelInfo, err
	}
	exitCode, err := b.runNonInteractive(ctx, req, args, stdout, stderr)
	return exitCode, modelInfo, err
}

// runInteractive runs Claude Code in interactive mode using a PTY.
func (b *ClaudeCode) runInteractive(ctx context.Context, req *pb.RunRequest, args []string, stdout, stderr io.Writer) (int32, error) {
	cmd := exec.CommandContext(ctx, b.BinaryPath, args...)

	opts := req.GetOptions()
	if opts != nil && opts.WorkDir != "" {
		cmd.Dir = opts.WorkDir
	}

	// Set environment variables
	cmd.Env = os.Environ()
	for k, v := range b.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	if opts != nil {
		for k, v := range opts.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	result, err := ptyrunner.RunInteractive(ctx, cmd, stdout, stderr)
	if err != nil {
		return 1, fmt.Errorf("failed to run claude: %w", err)
	}

	return int32(result.ExitCode), nil
}

// runNonInteractive runs Claude Code in non-interactive mode.
func (b *ClaudeCode) runNonInteractive(ctx context.Context, req *pb.RunRequest, args []string, stdout, stderr io.Writer) (int32, error) {
	cmd := exec.CommandContext(ctx, b.BinaryPath, args...)

	opts := req.GetOptions()
	if opts != nil && opts.WorkDir != "" {
		cmd.Dir = opts.WorkDir
	}

	// Set environment variables
	cmd.Env = os.Environ()
	for k, v := range b.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	if opts != nil {
		for k, v := range opts.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	result, err := ptyrunner.RunNonInteractive(ctx, cmd, stdout, stderr)
	if err != nil {
		return 1, fmt.Errorf("failed to run claude: %w", err)
	}

	return int32(result.ExitCode), nil
}

// buildArgs constructs the command-line arguments for claude.
func (b *ClaudeCode) buildArgs(req *pb.RunRequest) []string {
	// Start with configured args
	args := make([]string, len(b.Args))
	copy(args, b.Args)

	opts := req.GetOptions()

	// Add auto-approve flag
	if opts != nil && opts.AutoApprove {
		args = append(args, "--dangerously-skip-permissions")
	}

	// Add model selection
	if opts != nil && opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}

	// Add --print for oneshot mode
	if opts != nil && opts.Mode == pb.ExecutionMode_ONESHOT {
		args = append(args, "--print")
	}

	// Assemble context from fragments and pass via --append-system-prompt
	if ctx := b.AssembleContext(req.Fragments); ctx != "" {
		args = append(args, "--append-system-prompt", ctx)
	}

	// Add the prompt as the final argument
	if prompt := b.GetPromptContent(req); prompt != "" {
		args = append(args, prompt)
	}

	return args
}

// writeHooks loads hooks from config and writes them to .claude/settings.json.
func (b *ClaudeCode) writeHooks(workDir string, stderr io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Collect hooks from resolved profiles and top-level config
	hooks := b.collectHooks(cfg)
	if hooks == nil {
		return nil // No hooks configured
	}

	return WriteHooks(b.Name(), hooks, workDir)
}

// collectHooks collects hooks from config, merging profile hooks with top-level hooks.
// Always includes the auto-registered SessionStart hook for context injection.
func (b *ClaudeCode) collectHooks(cfg *config.Config) *config.HooksConfig {
	result := &config.HooksConfig{
		Plugins: make(map[string]config.BackendHooks),
	}

	// Auto-register SessionStart hook for context injection
	result.Unified.SessionStart = append(result.Unified.SessionStart, NewContextInjectionHook())

	// Start with top-level hooks
	mergeHooksConfig(result, &cfg.Hooks)

	// Add hooks from default profiles
	for _, profileName := range cfg.GetDefaultProfiles() {
		resolved, err := config.ResolveProfile(cfg.Profiles, profileName)
		if err != nil {
			continue
		}
		mergeHooksConfig(result, &resolved.Hooks)
	}

	return result
}

// mergeHooksConfig merges source hooks into target.
func mergeHooksConfig(target, source *config.HooksConfig) {
	// Merge unified hooks
	target.Unified.PreTool = append(target.Unified.PreTool, source.Unified.PreTool...)
	target.Unified.PostTool = append(target.Unified.PostTool, source.Unified.PostTool...)
	target.Unified.SessionStart = append(target.Unified.SessionStart, source.Unified.SessionStart...)
	target.Unified.SessionEnd = append(target.Unified.SessionEnd, source.Unified.SessionEnd...)
	target.Unified.PreShell = append(target.Unified.PreShell, source.Unified.PreShell...)
	target.Unified.PostFileEdit = append(target.Unified.PostFileEdit, source.Unified.PostFileEdit...)

	// Merge plugin-specific hooks
	for pluginName, backendHooks := range source.Plugins {
		if target.Plugins[pluginName] == nil {
			target.Plugins[pluginName] = make(config.BackendHooks)
		}
		for eventName, hooks := range backendHooks {
			target.Plugins[pluginName][eventName] = append(target.Plugins[pluginName][eventName], hooks...)
		}
	}
}

// isEmptyHooks returns true if no hooks are configured.
func isEmptyHooks(cfg *config.HooksConfig) bool {
	if len(cfg.Unified.PreTool) > 0 ||
		len(cfg.Unified.PostTool) > 0 ||
		len(cfg.Unified.SessionStart) > 0 ||
		len(cfg.Unified.SessionEnd) > 0 ||
		len(cfg.Unified.PreShell) > 0 ||
		len(cfg.Unified.PostFileEdit) > 0 {
		return false
	}
	for _, backendHooks := range cfg.Plugins {
		for _, hooks := range backendHooks {
			if len(hooks) > 0 {
				return false
			}
		}
	}
	return true
}

// loadPrompts loads all prompts from configured directories for slash command export.
func (b *ClaudeCode) loadPrompts(workDir string, stderr io.Writer) []*fragments.Fragment {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(stderr, "warning: failed to load config for prompts: %v\n", err)
		return nil
	}

	promptDirs := cfg.GetPromptDirs()
	if len(promptDirs) == 0 {
		return nil
	}

	loader := fragments.NewLoader(promptDirs, fragments.WithSuppressWarnings(true))
	infos, err := loader.List()
	if err != nil {
		fmt.Fprintf(stderr, "warning: failed to list prompts: %v\n", err)
		return nil
	}

	var prompts []*fragments.Fragment
	for _, info := range infos {
		frag, err := loader.Load(info.Name)
		if err != nil {
			continue
		}
		prompts = append(prompts, frag)
	}

	return prompts
}
