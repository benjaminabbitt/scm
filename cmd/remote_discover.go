package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/benjaminabbitt/scm/internal/remote"
)

var (
	discoverSource string
	discoverLimit  int
	discoverStars  int
)

var remoteDiscoverCmd = &cobra.Command{
	Use:   "discover [query]",
	Short: "Search GitHub/GitLab for SCM repositories",
	Long: `Discover SCM repositories on GitHub and GitLab.

Searches for repositories named 'scm' or starting with 'scm-'.
Only repositories with valid scm/v1/ structure are shown.

Examples:
  scm remote discover                      # Find all SCM repos
  scm remote discover golang               # Filter by 'golang' in description
  scm remote discover --source github      # Search GitHub only
  scm remote discover --stars 10           # Only repos with 10+ stars`,
	RunE: func(cmd *cobra.Command, args []string) error {
		query := ""
		if len(args) > 0 {
			query = strings.Join(args, " ")
		}

		auth := remote.LoadAuth("")

		// Determine which forges to search
		var forges []remote.ForgeType
		switch discoverSource {
		case "github":
			forges = []remote.ForgeType{remote.ForgeGitHub}
		case "gitlab":
			forges = []remote.ForgeType{remote.ForgeGitLab}
		default:
			forges = []remote.ForgeType{remote.ForgeGitHub, remote.ForgeGitLab}
		}

		// Search forges in parallel
		var wg sync.WaitGroup
		resultsCh := make(chan []remote.RepoInfo, len(forges))
		errorsCh := make(chan error, len(forges))

		for _, forge := range forges {
			wg.Add(1)
			go func(f remote.ForgeType) {
				defer wg.Done()

				var fetcher remote.Fetcher
				var err error

				switch f {
				case remote.ForgeGitHub:
					fmt.Printf("Searching GitHub...")
					fetcher = remote.NewGitHubFetcher(auth.GitHub)
				case remote.ForgeGitLab:
					fmt.Printf("Searching GitLab...")
					fetcher, err = remote.NewGitLabFetcher("", auth.GitLab)
					if err != nil {
						errorsCh <- fmt.Errorf("GitLab: %w", err)
						return
					}
				}

				repos, err := fetcher.SearchRepos(cmd.Context(), query, discoverLimit)
				if err != nil {
					errorsCh <- fmt.Errorf("%s: %w", f, err)
					fmt.Printf(" error\n")
					return
				}

				// Filter by stars
				filtered := repos[:0]
				for _, r := range repos {
					if r.Stars >= discoverStars {
						filtered = append(filtered, r)
					}
				}

				fmt.Printf(" found %d\n", len(filtered))
				resultsCh <- filtered
			}(forge)
		}

		wg.Wait()
		close(resultsCh)
		close(errorsCh)

		// Collect results
		var allRepos []remote.RepoInfo
		for repos := range resultsCh {
			allRepos = append(allRepos, repos...)
		}

		// Print errors
		for err := range errorsCh {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}

		if len(allRepos) == 0 {
			fmt.Println("\nNo SCM repositories found.")
			if query != "" {
				fmt.Printf("Try a different search term or remove the filter.\n")
			}
			return nil
		}

		// Display results
		fmt.Println()
		fmt.Printf("  # │ Forge  │ Repository          │ Stars │ Description\n")
		fmt.Printf("────┼────────┼─────────────────────┼───────┼─────────────────────────────────────\n")

		for i, r := range allRepos {
			forgeIcon := "GitHub"
			if r.Forge == remote.ForgeGitLab {
				forgeIcon = "GitLab"
			}

			// Truncate description
			desc := r.Description
			if len(desc) > 35 {
				desc = desc[:32] + "..."
			}

			repoName := fmt.Sprintf("%s/%s", r.Owner, r.Name)
			if len(repoName) > 19 {
				repoName = repoName[:16] + "..."
			}

			fmt.Printf("%3d │ %-6s │ %-19s │ %5d │ %s\n",
				i+1, forgeIcon, repoName, r.Stars, desc)
		}

		fmt.Println()

		// Interactive add
		if err := interactiveAdd(cmd.Context(), allRepos); err != nil {
			return err
		}

		return nil
	},
}

// interactiveAdd prompts the user to add a discovered repo as a remote.
func interactiveAdd(ctx context.Context, repos []remote.RepoInfo) error {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("Add remote? Enter number (or 'q' to quit): ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return nil // EOF is ok
		}

		input = strings.TrimSpace(input)
		if input == "q" || input == "" {
			return nil
		}

		num, err := strconv.Atoi(input)
		if err != nil || num < 1 || num > len(repos) {
			fmt.Printf("Invalid selection. Enter 1-%d or 'q'.\n", len(repos))
			continue
		}

		repo := repos[num-1]

		// Suggest name
		defaultName := repo.Owner
		fmt.Printf("Name for remote [%s]: ", defaultName)
		nameInput, _ := reader.ReadString('\n')
		name := strings.TrimSpace(nameInput)
		if name == "" {
			name = defaultName
		}

		// Add remote
		registry, err := remote.NewRegistry("")
		if err != nil {
			return fmt.Errorf("failed to initialize registry: %w", err)
		}

		if err := registry.Add(name, repo.URL); err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		fmt.Printf("Added remote '%s' → %s\n\n", name, repo.URL)
	}
}

func init() {
	remoteCmd.AddCommand(remoteDiscoverCmd)

	remoteDiscoverCmd.Flags().StringVarP(&discoverSource, "source", "s", "all",
		"Search specific forge: github, gitlab, or all")
	remoteDiscoverCmd.Flags().IntVarP(&discoverLimit, "limit", "n", 30,
		"Maximum results per forge")
	remoteDiscoverCmd.Flags().IntVar(&discoverStars, "stars", 0,
		"Minimum star count")
}
