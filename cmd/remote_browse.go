package cmd

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/benjaminabbitt/scm/internal/remote"
)

var browseRecursive bool

var remoteBundlesBrowseCmd = &cobra.Command{
	Use:   "browse <remote>",
	Short: "List available bundles in a remote",
	Long: `List bundles available in a remote repository.

Examples:
  scm remote bundles browse alice
  scm remote bundles browse alice --recursive`,
	Args: cobra.ExactArgs(1),
	RunE: runRemoteBrowse(remote.ItemTypeBundle),
}

var remoteProfilesBrowseCmd = &cobra.Command{
	Use:   "browse <remote>",
	Short: "List available profiles in a remote",
	Long: `List profiles available in a remote repository.

Examples:
  scm remote profiles browse alice
  scm remote profiles browse alice --recursive`,
	Args: cobra.ExactArgs(1),
	RunE: runRemoteBrowse(remote.ItemTypeProfile),
}

// runRemoteBrowse returns a RunE function for browsing items of the specified type.
func runRemoteBrowse(itemType remote.ItemType) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		remoteName := args[0]

		registry, err := remote.NewRegistry("")
		if err != nil {
			return fmt.Errorf("failed to initialize registry: %w", err)
		}

		rem, err := registry.Get(remoteName)
		if err != nil {
			return err
		}

		auth := remote.LoadAuth("")
		fetcher, err := remote.NewFetcher(rem.URL, auth)
		if err != nil {
			return fmt.Errorf("failed to create fetcher: %w", err)
		}

		owner, repo, err := remote.ParseRepoURL(rem.URL)
		if err != nil {
			return fmt.Errorf("invalid remote URL: %w", err)
		}

		// Get default branch
		branch, err := fetcher.GetDefaultBranch(cmd.Context(), owner, repo)
		if err != nil {
			return fmt.Errorf("failed to get default branch: %w", err)
		}

		// Build path to item type directory
		dirPath := fmt.Sprintf("scm/%s/%s", rem.Version, itemType.DirName())

		// List directory
		entries, err := listDirRecursive(cmd, fetcher, owner, repo, dirPath, branch, browseRecursive)
		if err != nil {
			return fmt.Errorf("failed to list %s: %w", itemType.Plural(), err)
		}

		if len(entries) == 0 {
			fmt.Printf("No %s found in %s\n", itemType.Plural(), remoteName)
			return nil
		}

		// Display results
		plural := itemType.Plural()
		title := strings.ToUpper(plural[:1]) + plural[1:]
		fmt.Printf("%s in %s (%s):\n\n", title, remoteName, rem.URL)

		// Sort entries
		sort.Strings(entries)

		for _, entry := range entries {
			// Remove .yaml extension and directory prefix
			name := strings.TrimSuffix(entry, ".yaml")
			name = strings.TrimPrefix(name, dirPath+"/")
			fmt.Printf("  %s/%s\n", remoteName, name)
		}

		fmt.Println()
		fmt.Printf("Pull with: scm remote %s pull %s/<name>\n", itemType.Plural(), remoteName)

		return nil
	}
}

// listDirRecursive lists directory contents, optionally recursively.
func listDirRecursive(cmd *cobra.Command, fetcher remote.Fetcher, owner, repo, path, ref string, recursive bool) ([]string, error) {
	entries, err := fetcher.ListDir(cmd.Context(), owner, repo, path, ref)
	if err != nil {
		return nil, err
	}

	var results []string
	for _, entry := range entries {
		fullPath := filepath.Join(path, entry.Name)
		if entry.IsDir {
			if recursive {
				subEntries, err := listDirRecursive(cmd, fetcher, owner, repo, fullPath, ref, true)
				if err != nil {
					// Continue on error for subdirectories
					continue
				}
				results = append(results, subEntries...)
			}
		} else if strings.HasSuffix(entry.Name, ".yaml") {
			results = append(results, fullPath)
		}
	}

	return results, nil
}

func init() {
	// Add browse to bundle and profile commands
	remoteBundlesCmd.AddCommand(remoteBundlesBrowseCmd)
	remoteProfilesCmd.AddCommand(remoteProfilesBrowseCmd)

	// Flags for browse commands
	for _, cmd := range []*cobra.Command{
		remoteBundlesBrowseCmd,
		remoteProfilesBrowseCmd,
	} {
		cmd.Flags().BoolVarP(&browseRecursive, "recursive", "r", true,
			"List items in subdirectories")
	}
}
