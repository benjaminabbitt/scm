package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/benjaminabbitt/scm/internal/remote"
)

var remoteBundlesSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search for bundles across configured remotes",
	Long: `Search for bundles across all configured remotes.

Query syntax:
  Plain text         Full-text search on name and description
  tag:foo/bar        Tags with AND (default)
  tag:foo/bar/OR     Tags with OR
  tag:foo/NOT        Negated tag
  author:name        Filter by author
  version:spec       Version constraint

Examples:
  scm remote bundles search go-tools
  scm remote bundles search "tag:golang/testing"
  scm remote bundles search "tag:security author:alice"`,
	Args: cobra.MinimumNArgs(1),
	RunE: runRemoteSearch(remote.ItemTypeBundle),
}

var remoteProfilesSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search for profiles across configured remotes",
	Long: `Search for profiles across all configured remotes.

Query syntax is the same as bundle search.

Examples:
  scm remote profiles search security
  scm remote profiles search "tag:enterprise"`,
	Args: cobra.MinimumNArgs(1),
	RunE: runRemoteSearch(remote.ItemTypeProfile),
}

// runRemoteSearch returns a RunE function for searching items of the specified type.
func runRemoteSearch(itemType remote.ItemType) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		queryStr := strings.Join(args, " ")
		query := remote.ParseSearchQuery(queryStr)

		registry, err := remote.NewRegistry("")
		if err != nil {
			return fmt.Errorf("failed to initialize registry: %w", err)
		}

		remotes := registry.List()
		if len(remotes) == 0 {
			fmt.Println("No remotes configured. Add one with: scm remote add <name> <url>")
			return nil
		}

		auth := remote.LoadAuth("")

		// Search all remotes in parallel
		var wg sync.WaitGroup
		resultsCh := make(chan []remote.SearchResult, len(remotes))
		errorsCh := make(chan error, len(remotes))

		for _, rem := range remotes {
			wg.Add(1)
			go func(r *remote.Remote) {
				defer wg.Done()

				results, err := searchRemote(cmd.Context(), r, itemType, query, auth)
				if err != nil {
					errorsCh <- fmt.Errorf("%s: %w", r.Name, err)
					return
				}
				resultsCh <- results
			}(rem)
		}

		wg.Wait()
		close(resultsCh)
		close(errorsCh)

		// Collect results
		var allResults []remote.SearchResult
		for results := range resultsCh {
			allResults = append(allResults, results...)
		}

		// Print errors
		for err := range errorsCh {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}

		if len(allResults) == 0 {
			fmt.Printf("No %s found matching: %s\n", itemType.Plural(), queryStr)
			return nil
		}

		// Display results
		fmt.Printf("Found %d %s matching: %s\n\n", len(allResults), itemType.Plural(), queryStr)
		fmt.Printf("  %-12s │ %-20s │ %-30s │ %s\n", "Remote", "Name", "Tags", "Author")
		fmt.Printf("──────────────┼──────────────────────┼────────────────────────────────┼────────────\n")

		for _, r := range allResults {
			tags := strings.Join(r.Entry.Tags, ", ")
			if len(tags) > 28 {
				tags = tags[:25] + "..."
			}

			name := r.Entry.Name
			if len(name) > 18 {
				name = name[:15] + "..."
			}

			fmt.Printf("  %-12s │ %-20s │ %-30s │ %s\n",
				r.Remote, name, tags, r.Entry.Author)
		}

		fmt.Println()
		fmt.Printf("Pull with: scm remote %s pull <remote>/<name>\n", itemType.Plural())

		return nil
	}
}

// searchRemote searches a single remote for matching items.
func searchRemote(ctx context.Context, rem *remote.Remote, itemType remote.ItemType, query remote.SearchQuery, auth remote.AuthConfig) ([]remote.SearchResult, error) {
	fetcher, err := remote.NewFetcher(rem.URL, auth)
	if err != nil {
		return nil, err
	}

	owner, repo, err := remote.ParseRepoURL(rem.URL)
	if err != nil {
		return nil, err
	}

	// Get default branch
	branch, err := fetcher.GetDefaultBranch(ctx, owner, repo)
	if err != nil {
		return nil, err
	}

	// Try to fetch manifest first (faster)
	manifestPath := fmt.Sprintf("scm/%s/manifest.yaml", rem.Version)
	manifestContent, err := fetcher.FetchFile(ctx, owner, repo, manifestPath, branch)
	if err == nil {
		// Parse manifest and search
		return searchManifest(rem, manifestContent, itemType, query)
	}

	// Fall back to directory listing
	return searchDirectory(ctx, fetcher, rem, owner, repo, branch, itemType, query)
}

// searchManifest searches the manifest for matching items.
func searchManifest(rem *remote.Remote, content []byte, itemType remote.ItemType, query remote.SearchQuery) ([]remote.SearchResult, error) {
	var manifest remote.Manifest
	if err := yaml.Unmarshal(content, &manifest); err != nil {
		return nil, err
	}

	var entries []remote.ManifestEntry
	switch itemType {
	case remote.ItemTypeBundle:
		entries = manifest.Bundles
	case remote.ItemTypeProfile:
		entries = manifest.Profiles
	}

	var results []remote.SearchResult
	for _, entry := range entries {
		if remote.MatchesQuery(entry, query) {
			results = append(results, remote.SearchResult{
				Remote:    rem.Name,
				Entry:     entry,
				RemoteURL: rem.URL,
			})
		}
	}

	return results, nil
}

// searchDirectory searches by listing directory contents.
func searchDirectory(ctx context.Context, fetcher remote.Fetcher, rem *remote.Remote, owner, repo, branch string, itemType remote.ItemType, query remote.SearchQuery) ([]remote.SearchResult, error) {
	dirPath := fmt.Sprintf("scm/%s/%s", rem.Version, itemType.DirName())

	entries, err := fetcher.ListDir(ctx, owner, repo, dirPath, branch)
	if err != nil {
		return nil, err
	}

	var results []remote.SearchResult
	for _, entry := range entries {
		if entry.IsDir || !strings.HasSuffix(entry.Name, ".yaml") {
			continue
		}

		name := strings.TrimSuffix(entry.Name, ".yaml")

		// Create a minimal manifest entry for matching
		manifestEntry := remote.ManifestEntry{
			Name: name,
		}

		// Only do text matching without fetching full content
		if remote.MatchesQuery(manifestEntry, query) {
			results = append(results, remote.SearchResult{
				Remote:    rem.Name,
				Entry:     manifestEntry,
				RemoteURL: rem.URL,
			})
		}
	}

	return results, nil
}

func init() {
	// Add search to bundle and profile commands
	remoteBundlesCmd.AddCommand(remoteBundlesSearchCmd)
	remoteProfilesCmd.AddCommand(remoteProfilesSearchCmd)
}
