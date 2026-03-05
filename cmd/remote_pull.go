package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/benjaminabbitt/scm/internal/remote"
)

var pullForce bool
var pullCascade bool
var pullNoCascade bool

// remoteProfilesCmd is the parent for profile-specific remote commands.
var remoteProfilesCmd = &cobra.Command{
	Use:   "profiles",
	Short: "Manage remote profiles",
	Long:  `Commands for searching, browsing, and pulling profiles from remote sources.`,
}

var remoteProfilesPullCmd = &cobra.Command{
	Use:   "pull <remote/path[@ref]>",
	Short: "Download a profile from a remote source",
	Long: `Download a profile from a remote source with security review.

The full content of the profile will be displayed before installation.
You must review and explicitly confirm before the profile is installed.

By default, all bundles referenced by the profile are also pulled (cascade).
Use --no-cascade to only pull the profile itself.

Examples:
  scm remote profiles pull alice/security-focused
  scm remote profiles pull corp/enterprise@v1.0.0
  scm remote profiles pull alice/go-developer --no-cascade`,
	Args: cobra.ExactArgs(1),
	RunE: runRemotePull(remote.ItemTypeProfile),
}

// remoteBundlesCmd is the parent for bundle-specific remote commands.
var remoteBundlesCmd = &cobra.Command{
	Use:   "bundles",
	Short: "Manage remote bundles",
	Long:  `Commands for searching, browsing, and pulling bundles from remote sources.`,
}

var remoteBundlesPullCmd = &cobra.Command{
	Use:   "pull <remote/path[@ref]>",
	Short: "Download a bundle from a remote source",
	Long: `Download a bundle from a remote source with security review.

Bundles combine fragments, prompts, and optionally MCP server configurations.
The full content will be displayed before installation.
You must review and explicitly confirm before the bundle is installed.

WARNING: Bundles with MCP servers execute commands on your system.
Only install bundles from sources you trust.

Examples:
  scm remote bundles pull github:bundles/core-practices
  scm remote bundles pull github:bundles/go-development@v1.0.0`,
	Args: cobra.ExactArgs(1),
	RunE: runRemotePull(remote.ItemTypeBundle),
}

// runRemotePull returns a RunE function for pulling items of the specified type.
func runRemotePull(itemType remote.ItemType) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		refStr := args[0]

		registry, err := remote.NewRegistry("")
		if err != nil {
			return fmt.Errorf("failed to initialize registry: %w", err)
		}

		auth := remote.LoadAuth("")
		puller := remote.NewPuller(registry, auth)

		// Cascade is enabled by default for profiles, disabled for others
		cascade := pullCascade
		if itemType == remote.ItemTypeProfile && !pullNoCascade {
			cascade = true
		}

		opts := remote.PullOptions{
			Force:    pullForce,
			ItemType: itemType,
			Cascade:  cascade,
		}

		result, err := puller.Pull(cmd.Context(), refStr, opts)
		if err != nil {
			return err
		}

		action := "Installed"
		if result.Overwritten {
			action = "Updated"
		}

		fmt.Printf("%s %s → %s\n", action, refStr, result.LocalPath)
		fmt.Printf("SHA: %s\n", result.SHA[:7])

		if len(result.CascadePulled) > 0 {
			fmt.Printf("Cascade pulled %d bundles\n", len(result.CascadePulled))
		}

		return nil
	}
}

func init() {
	// Add type-specific subcommands to remote
	remoteCmd.AddCommand(remoteBundlesCmd)
	remoteCmd.AddCommand(remoteProfilesCmd)

	// Add pull to each type
	remoteBundlesCmd.AddCommand(remoteBundlesPullCmd)
	remoteProfilesCmd.AddCommand(remoteProfilesPullCmd)

	// Flags for pull commands
	for _, cmd := range []*cobra.Command{
		remoteBundlesPullCmd,
		remoteProfilesPullCmd,
	} {
		cmd.Flags().BoolVarP(&pullForce, "force", "f", false,
			"Skip confirmation prompt (content still displayed)")
	}

	// Cascade flag for profiles (enabled by default)
	remoteProfilesPullCmd.Flags().BoolVar(&pullNoCascade, "no-cascade", false,
		"Don't pull referenced bundles")
	remoteProfilesPullCmd.Flags().BoolVar(&pullCascade, "cascade", true,
		"Pull all referenced bundles (default for profiles)")

	// Cascade flag for bundles (disabled by default, future use)
	remoteBundlesPullCmd.Flags().BoolVar(&pullCascade, "cascade", false,
		"Pull referenced dependencies")
}
