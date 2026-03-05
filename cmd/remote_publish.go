package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/benjaminabbitt/scm/internal/remote"
)

var (
	publishPR      bool
	publishBranch  string
	publishMessage string
	publishVersion string
)

var remotePublishCmd = &cobra.Command{
	Use:   "publish",
	Short: "Publish local items to remote repositories",
	Long:  `Commands for publishing bundles and profiles to remote sources.`,
}

var remoteBundlesPublishCmd = &cobra.Command{
	Use:   "publish <local-file> <remote>",
	Short: "Publish a bundle to a remote repository",
	Long: `Publish a local bundle to a remote repository.

By default, publishes directly to the default branch. Use --pr to create
a pull request instead.

Examples:
  scm remote bundles publish .scm/bundles/go-tools.yaml alice
  scm remote bundles publish go-tools.yaml alice --pr
  scm remote bundles publish go-tools.yaml alice --branch main --message "Add go-tools bundle"`,
	Args: cobra.ExactArgs(2),
	RunE: runRemotePublish(remote.ItemTypeBundle),
}

var remoteProfilesPublishCmd = &cobra.Command{
	Use:   "publish <local-file> <remote>",
	Short: "Publish a profile to a remote repository",
	Long: `Publish a local profile to a remote repository.

By default, publishes directly to the default branch. Use --pr to create
a pull request instead.

Examples:
  scm remote profiles publish .scm/profiles/performance.yaml alice
  scm remote profiles publish performance.yaml alice --pr`,
	Args: cobra.ExactArgs(2),
	RunE: runRemotePublish(remote.ItemTypeProfile),
}

func runRemotePublish(itemType remote.ItemType) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		localPath := args[0]
		remoteName := args[1]

		// Resolve local path
		absPath, err := filepath.Abs(localPath)
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}

		// Check file exists
		if _, err := os.Stat(absPath); err != nil {
			return fmt.Errorf("file not found: %s", localPath)
		}

		// Initialize registry and auth
		registry, err := remote.NewRegistry("")
		if err != nil {
			return fmt.Errorf("failed to initialize registry: %w", err)
		}

		auth := remote.LoadAuth("")

		// Build options
		opts := remote.PublishOptions{
			CreatePR: publishPR,
			Branch:   publishBranch,
			Message:  publishMessage,
			Version:  publishVersion,
			ItemType: itemType,
		}

		// Publish
		fmt.Printf("Publishing %s to %s...\n", filepath.Base(localPath), remoteName)

		result, err := remote.Publish(cmd.Context(), absPath, remoteName, opts, registry, auth)
		if err != nil {
			return err
		}

		// Report result
		if result.PRURL != "" {
			fmt.Printf("Created pull request: %s\n", result.PRURL)
		} else {
			action := "Created"
			if !result.Created {
				action = "Updated"
			}
			fmt.Printf("%s %s\n", action, result.Path)
			fmt.Printf("Commit: %s\n", result.SHA[:7])
		}

		return nil
	}
}

func init() {
	remoteCmd.AddCommand(remotePublishCmd)

	// Add publish subcommands to type-specific commands
	remoteBundlesCmd.AddCommand(remoteBundlesPublishCmd)
	remoteProfilesCmd.AddCommand(remoteProfilesPublishCmd)

	// Add flags to all publish commands
	for _, cmd := range []*cobra.Command{
		remoteBundlesPublishCmd,
		remoteProfilesPublishCmd,
	} {
		cmd.Flags().BoolVar(&publishPR, "pr", false,
			"Create a pull request instead of pushing directly")
		cmd.Flags().StringVar(&publishBranch, "branch", "",
			"Target branch (default: repository default branch)")
		cmd.Flags().StringVarP(&publishMessage, "message", "m", "",
			"Commit message")
		cmd.Flags().StringVar(&publishVersion, "version", "",
			"SCM version directory (default: v1)")
	}
}
