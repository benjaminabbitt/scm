package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/benjaminabbitt/scm/internal/remote"
)

var remoteCmd = &cobra.Command{
	Use:   "remote",
	Short: "Manage remote fragment/prompt sources",
	Long: `Manage remote sources for fragments and prompts.

Remote sources are Git repositories (GitHub/GitLab) containing shared
fragments, prompts, and profiles. Use 'scm remote discover' to find
public repositories, and 'scm remote add' to register them.

Examples:
  scm remote add alice alice/scm              # Add GitHub remote (shorthand)
  scm remote add corp https://gitlab.com/corp/scm  # Add GitLab remote
  scm remote list                             # List configured remotes
  scm remote remove alice                     # Remove a remote`,
}

var remoteAddCmd = &cobra.Command{
	Use:   "add <name> <url>",
	Short: "Register a remote source",
	Long: `Register a remote repository as a source for fragments and prompts.

URL formats:
  alice/scm                      GitHub shorthand (expands to https://github.com/alice/scm)
  https://github.com/alice/scm   Full GitHub URL
  https://gitlab.com/corp/scm   Full GitLab URL
  git@github.com:alice/scm.git   SSH URL (converted to HTTPS)

Examples:
  scm remote add alice alice/scm
  scm remote add corp https://gitlab.com/corp/scm`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		url := args[1]

		registry, err := remote.NewRegistry("")
		if err != nil {
			return fmt.Errorf("failed to initialize registry: %w", err)
		}

		if err := registry.Add(name, url); err != nil {
			return err
		}

		// Verify the remote is valid
		auth := remote.LoadAuth("")
		fetcher, err := registry.GetFetcher(name, auth)
		if err != nil {
			// Rollback
			registry.Remove(name)
			return fmt.Errorf("failed to create fetcher: %w", err)
		}

		owner, repo, err := remote.ParseRepoURL(url)
		if err != nil {
			registry.Remove(name)
			return fmt.Errorf("invalid URL: %w", err)
		}

		// Check if repo has valid SCM structure
		valid, err := fetcher.ValidateRepo(cmd.Context(), owner, repo)
		if err != nil {
			fmt.Printf("Warning: could not validate repository structure: %v\n", err)
		} else if !valid {
			fmt.Printf("Warning: repository does not have scm/v1/ directory structure\n")
		}

		r, _ := registry.Get(name)
		fmt.Printf("Added remote '%s' → %s\n", name, r.URL)
		return nil
	},
}

var remoteRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"rm"},
	Short:   "Remove a remote source",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		registry, err := remote.NewRegistry("")
		if err != nil {
			return fmt.Errorf("failed to initialize registry: %w", err)
		}

		if err := registry.Remove(name); err != nil {
			return err
		}

		fmt.Printf("Removed remote '%s'\n", name)
		return nil
	},
}

var remoteListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List configured remotes",
	RunE: func(cmd *cobra.Command, args []string) error {
		registry, err := remote.NewRegistry("")
		if err != nil {
			return fmt.Errorf("failed to initialize registry: %w", err)
		}

		remotes := registry.List()
		if len(remotes) == 0 {
			fmt.Println("No remotes configured.")
			fmt.Println("Use 'scm remote add <name> <url>' to add a remote.")
			fmt.Println("Use 'scm remote discover' to find public repositories.")
			return nil
		}

		fmt.Println("Configured remotes:")
		for _, r := range remotes {
			fmt.Printf("  %-15s %s (version: %s)\n", r.Name, r.URL, r.Version)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(remoteCmd)

	remoteCmd.AddCommand(remoteAddCmd)
	remoteCmd.AddCommand(remoteRemoveCmd)
	remoteCmd.AddCommand(remoteListCmd)
}
