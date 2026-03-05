package backends

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// Codex implements the Backend interface for OpenAI Codex CLI.
//
// DISCLAIMER: This plugin is untested and provided on a best-effort basis.
type Codex struct {
	BaseBackend
	context *CLIContextProvider
}

// NewCodex creates a new Codex backend with default settings.
func NewCodex() *Codex {
	b := &Codex{
		BaseBackend: NewBaseBackend("codex", "1.0.0"),
		context:     &CLIContextProvider{},
	}
	b.BinaryPath = "codex"
	return b
}

// Lifecycle returns nil - Codex doesn't support lifecycle hooks.
func (b *Codex) Lifecycle() LifecycleHandler { return nil }

// Skills returns nil - Codex doesn't support skills.
func (b *Codex) Skills() SkillRegistry { return nil }

// Context returns the context provider (CLI arg injection).
func (b *Codex) Context() ContextProvider { return b.context }

// MCP returns nil - Codex doesn't support MCP servers.
func (b *Codex) MCP() MCPManager { return nil }

// Setup prepares the backend for execution.
func (b *Codex) Setup(ctx context.Context, req *SetupRequest) error {
	b.SetWorkDir(req.WorkDir)
	if _, err := WriteContextFile(b.WorkDir(), req.Fragments); err != nil {
		return fmt.Errorf("failed to write context file: %w", err)
	}
	return b.context.Provide(b.WorkDir(), req.Fragments)
}

// Execute runs the backend with the given request.
func (b *Codex) Execute(ctx context.Context, req *ExecuteRequest, stdout, stderr io.Writer) (*ExecuteResult, error) {
	modelName := req.Model
	if modelName == "" {
		modelName = "o3-mini"
	}
	modelInfo := &ModelInfo{ModelName: modelName, Provider: "openai"}

	if req.DryRun {
		return &ExecuteResult{ExitCode: 0, ModelInfo: modelInfo}, nil
	}

	quiet := req.Mode == ModeOneshot
	args := b.buildArgs(req, quiet)
	if req.Verbosity >= 16 {
		fmt.Fprintf(stderr, "[v16] %s %s\n", b.BinaryPath, strings.Join(args, " "))
	}

	var exitCode int32
	var err error
	if req.Mode == ModeInteractive {
		exitCode, err = b.RunInteractive(ctx, args, req.Env, stdout, stderr)
	} else {
		exitCode, err = b.RunNonInteractive(ctx, args, req.Env, stdout, stderr)
	}

	return &ExecuteResult{ExitCode: exitCode, ModelInfo: modelInfo}, err
}

// Cleanup releases resources after execution.
func (b *Codex) Cleanup(ctx context.Context) error { return nil }

func (b *Codex) buildArgs(req *ExecuteRequest, quiet bool) []string {
	args := make([]string, len(b.Args))
	copy(args, b.Args)

	if req.AutoApprove {
		args = append(args, "--full-auto")
	}
	if quiet {
		args = append(args, "--quiet")
	}

	context := b.context.GetAssembled()
	prompt := GetPromptContent(req.Prompt)
	if prompt != "" {
		var message string
		if context != "" {
			message = fmt.Sprintf("Context:\n%s\n\n---\n\nTask: %s", context, prompt)
		} else {
			message = prompt
		}
		args = append(args, message)
	}

	return args
}
