package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/benjaminabbitt/scm/internal/gitutil"
	"github.com/benjaminabbitt/scm/internal/lm/backends"
)

// HookOutput represents the JSON output format for AI tool hooks.
// This format is compatible with both Claude Code and Gemini CLI SessionStart hooks.
type HookOutput struct {
	HookSpecificOutput *HookSpecificOutput `json:"hookSpecificOutput,omitempty"`
}

// HookSpecificOutput contains hook-specific data to inject.
type HookSpecificOutput struct {
	AdditionalContext string `json:"additionalContext,omitempty"`
}

var hookInjectContextCmd = &cobra.Command{
	Use:   "inject-context",
	Short: "Inject session context for AI tool hooks",
	Long: `Reads the session context file identified by SCM_CONTEXT_ID environment variable
and outputs JSON suitable for AI tool SessionStart hooks.

This command is invoked automatically by AI tools (Claude Code, Gemini CLI) during
their SessionStart event to inject fresh context on startup, resume, or /clear.

Output format (JSON to stdout):
{
  "hookSpecificOutput": {
    "additionalContext": "<context content>"
  }
}`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Always output valid JSON, even on errors.
		// This ensures Claude doesn't hang waiting for output.
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "scm hook inject-context: panic: %v\n", r)
				// Output empty JSON on panic
				fmt.Println("{}")
			}
		}()

		// Determine work directory (git root or current directory)
		workDir := "."
		if root, err := gitutil.FindRoot("."); err == nil {
			workDir = root
		}

		// Read session context from file
		content, err := backends.ReadSessionContext(workDir)
		if err != nil {
			// Log to stderr, output empty JSON to stdout
			fmt.Fprintf(os.Stderr, "scm hook inject-context: warning: failed to read session context: %v\n", err)
			content = ""
		}

		// Build output
		output := HookOutput{}
		if content != "" {
			output.HookSpecificOutput = &HookSpecificOutput{
				AdditionalContext: content,
			}
		}

		// Output JSON to stdout
		encoder := json.NewEncoder(os.Stdout)
		if err := encoder.Encode(output); err != nil {
			// If encoding fails, output empty JSON
			fmt.Fprintf(os.Stderr, "scm hook inject-context: warning: failed to encode output: %v\n", err)
			fmt.Println("{}")
		}
		return nil
	},
}

func init() {
	hookCmd.AddCommand(hookInjectContextCmd)
}
