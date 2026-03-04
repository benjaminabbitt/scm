package cmd

import (
	"github.com/spf13/cobra"
)

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Hook commands for AI tool integration",
	Long: `Hook commands are invoked by AI tools (Claude Code, Gemini CLI, etc.)
during their lifecycle events. These commands are not intended for direct user invocation.`,
}

func init() {
	rootCmd.AddCommand(hookCmd)
}
