package backends

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// QDeveloper implements the Backend interface for Amazon Q Developer CLI.
//
// DISCLAIMER: This plugin is untested and provided on a best-effort basis.
type QDeveloper struct {
	BaseBackend
	context *CLIContextProvider
}

// NewQDeveloper creates a new Q Developer backend with default settings.
func NewQDeveloper() *QDeveloper {
	b := &QDeveloper{
		BaseBackend: NewBaseBackend("q", "1.0.0"),
		context:     &CLIContextProvider{},
	}
	b.BinaryPath = "q"
	return b
}

// Lifecycle returns nil - QDeveloper doesn't support lifecycle hooks.
func (b *QDeveloper) Lifecycle() LifecycleHandler { return nil }

// Skills returns nil - QDeveloper doesn't support skills.
func (b *QDeveloper) Skills() SkillRegistry { return nil }

// Context returns the context provider (CLI arg injection).
func (b *QDeveloper) Context() ContextProvider { return b.context }

// MCP returns nil - QDeveloper doesn't support MCP servers.
func (b *QDeveloper) MCP() MCPManager { return nil }

// Setup prepares the backend for execution.
func (b *QDeveloper) Setup(ctx context.Context, req *SetupRequest) error {
	b.SetWorkDir(req.WorkDir)
	if _, err := WriteContextFile(b.WorkDir(), req.Fragments); err != nil {
		return fmt.Errorf("failed to write context file: %w", err)
	}
	return b.context.Provide(b.WorkDir(), req.Fragments)
}

// Execute runs the backend with the given request.
func (b *QDeveloper) Execute(ctx context.Context, req *ExecuteRequest, stdout, stderr io.Writer) (*ExecuteResult, error) {
	modelName := req.Model
	if modelName == "" {
		modelName = "amazon-q"
	}
	modelInfo := &ModelInfo{ModelName: modelName, Provider: "amazon"}

	if req.DryRun {
		return &ExecuteResult{ExitCode: 0, ModelInfo: modelInfo}, nil
	}

	args := b.buildArgs(req)
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
func (b *QDeveloper) Cleanup(ctx context.Context) error { return nil }

func (b *QDeveloper) buildArgs(req *ExecuteRequest) []string {
	args := make([]string, len(b.Args))
	copy(args, b.Args)

	// Q Developer uses "chat" subcommand
	args = append(args, "chat")

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
