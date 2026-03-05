package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/benjaminabbitt/scm/internal/remote"
)

var remoteLockCmd = &cobra.Command{
	Use:   "lock",
	Short: "Generate lockfile from installed remote items",
	Long: `Generate a lockfile (.scm/lock.yaml) from currently installed remote items.

The lockfile pins exact versions (commit SHAs) of remote dependencies for
reproducible installations. Commit this file to your repository.

Examples:
  scm remote lock              # Generate lockfile from installed items`,
	RunE: runRemoteLock,
}

var remoteInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install items from lockfile",
	Long: `Install all items specified in the lockfile (.scm/lock.yaml).

This is useful for CI/CD pipelines and setting up new development environments
with exact versions of remote dependencies.

Examples:
  scm remote install           # Install from lockfile
  scm remote install --force   # Skip confirmation prompts`,
	RunE: runRemoteInstall,
}

var remoteOutdatedCmd = &cobra.Command{
	Use:   "outdated",
	Short: "Show items with newer versions available",
	Long: `Check if any locked items have newer versions available.

Compares locked SHAs against the latest commits on the default branch
of each remote.

Examples:
  scm remote outdated`,
	RunE: runRemoteOutdated,
}

func runRemoteLock(cmd *cobra.Command, args []string) error {
	baseDir := ".scm"

	lockManager := remote.NewLockfileManager(baseDir)
	lockfile := &remote.Lockfile{
		Version:  1,
		Bundles:  make(map[string]remote.LockEntry),
		Profiles: make(map[string]remote.LockEntry),
	}

	itemCount := 0

	// Scan for installed items in project .scm directory (bundles and profiles only)
	for _, itemType := range []remote.ItemType{
		remote.ItemTypeBundle,
		remote.ItemTypeProfile,
	} {
		var dirName string
		switch itemType {
		case remote.ItemTypeBundle:
			dirName = "bundles"
		case remote.ItemTypeProfile:
			dirName = "profiles"
		}

		itemDir := filepath.Join(baseDir, dirName)
		entries, err := os.ReadDir(itemDir)
		if err != nil {
			continue // Directory doesn't exist
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			remoteName := entry.Name()
			remoteDir := filepath.Join(itemDir, remoteName)

			// Find yaml files in this remote's directory
			files, err := filepath.Glob(filepath.Join(remoteDir, "**", "*.yaml"))
			if err != nil {
				continue
			}
			// Also get files in the root of the remote directory
			rootFiles, _ := filepath.Glob(filepath.Join(remoteDir, "*.yaml"))
			files = append(files, rootFiles...)

			for _, file := range files {
				content, err := os.ReadFile(file)
				if err != nil {
					continue
				}

				// Try to extract source metadata
				var meta struct {
					Source remote.SourceMeta `yaml:"_source"`
				}
				if err := yaml.Unmarshal(content, &meta); err != nil {
					continue
				}

				if meta.Source.SHA == "" {
					continue // No source metadata
				}

				// Build reference key
				relPath, _ := filepath.Rel(filepath.Join(itemDir, remoteName), file)
				name := strings.TrimSuffix(relPath, ".yaml")
				ref := fmt.Sprintf("%s/%s", remoteName, name)

				lockEntry := remote.LockEntry{
					SHA:        meta.Source.SHA,
					URL:        meta.Source.URL,
					SCMVersion: meta.Source.Version,
					FetchedAt:  meta.Source.FetchedAt,
				}

				lockfile.AddEntry(itemType, ref, lockEntry)
				itemCount++
			}
		}
	}

	if itemCount == 0 {
		fmt.Println("No remote items with source metadata found.")
		fmt.Println("Pull items with: scm remote [bundles|profiles] pull <remote>/<name>")
		return nil
	}

	if err := lockManager.Save(lockfile); err != nil {
		return err
	}

	fmt.Printf("Generated %s with %d entries\n", lockManager.Path(), itemCount)
	fmt.Println("Commit this file to your repository for reproducible installations.")

	return nil
}

func runRemoteInstall(cmd *cobra.Command, args []string) error {
	lockManager := remote.NewLockfileManager(".scm")
	lockfile, err := lockManager.Load()
	if err != nil {
		return err
	}

	if lockfile.IsEmpty() {
		fmt.Println("No entries in lockfile.")
		fmt.Println("Generate one with: scm remote lock")
		return nil
	}

	registry, err := remote.NewRegistry("")
	if err != nil {
		return fmt.Errorf("failed to initialize registry: %w", err)
	}

	auth := remote.LoadAuth("")
	puller := remote.NewPuller(registry, auth)

	entries := lockfile.AllEntries()
	fmt.Printf("Installing %d items from lockfile...\n\n", len(entries))

	installed := 0
	skipped := 0
	failed := 0

	for _, e := range entries {
		ref := fmt.Sprintf("%s@%s", e.Ref, e.Entry.SHA[:7])
		fmt.Printf("Installing %s %s...", e.Type, ref)

		opts := remote.PullOptions{
			Force:    pullForce,
			ItemType: e.Type,
		}

		result, err := puller.Pull(cmd.Context(), ref, opts)
		if err != nil {
			if strings.Contains(err.Error(), "cancelled") {
				fmt.Println(" skipped")
				skipped++
			} else {
				fmt.Printf(" error: %v\n", err)
				failed++
			}
			continue
		}

		action := "installed"
		if result.Overwritten {
			action = "updated"
		}
		fmt.Printf(" %s\n", action)
		installed++
	}

	fmt.Println()
	fmt.Printf("Installed: %d, Skipped: %d, Failed: %d\n", installed, skipped, failed)

	return nil
}

func runRemoteOutdated(cmd *cobra.Command, args []string) error {
	lockManager := remote.NewLockfileManager(".scm")
	lockfile, err := lockManager.Load()
	if err != nil {
		return err
	}

	if lockfile.IsEmpty() {
		fmt.Println("No entries in lockfile.")
		return nil
	}

	registry, err := remote.NewRegistry("")
	if err != nil {
		return fmt.Errorf("failed to initialize registry: %w", err)
	}

	auth := remote.LoadAuth("")

	entries := lockfile.AllEntries()
	fmt.Printf("Checking %d items for updates...\n\n", len(entries))

	var outdated []struct {
		Type      remote.ItemType
		Ref       string
		LockedSHA string
		LatestSHA string
		Age       time.Duration
	}

	for _, e := range entries {
		ref, err := remote.ParseReference(e.Ref)
		if err != nil {
			continue
		}

		rem, err := registry.Get(ref.Remote)
		if err != nil {
			continue
		}

		fetcher, err := remote.NewFetcher(rem.URL, auth)
		if err != nil {
			continue
		}

		owner, repo, err := remote.ParseRepoURL(rem.URL)
		if err != nil {
			continue
		}

		latestSHA, err := getLatestSHA(cmd.Context(), fetcher, owner, repo)
		if err != nil {
			continue
		}

		if latestSHA != e.Entry.SHA {
			outdated = append(outdated, struct {
				Type      remote.ItemType
				Ref       string
				LockedSHA string
				LatestSHA string
				Age       time.Duration
			}{
				Type:      e.Type,
				Ref:       e.Ref,
				LockedSHA: e.Entry.SHA,
				LatestSHA: latestSHA,
				Age:       time.Since(e.Entry.FetchedAt),
			})
		}
	}

	if len(outdated) == 0 {
		fmt.Println("All items are up to date!")
		return nil
	}

	fmt.Printf("Found %d outdated items:\n\n", len(outdated))
	fmt.Printf("  %-10s │ %-25s │ %-10s │ %-10s │ %s\n", "Type", "Reference", "Locked", "Latest", "Age")
	fmt.Printf("────────────┼───────────────────────────┼────────────┼────────────┼────────────\n")

	for _, o := range outdated {
		lockedShort := o.LockedSHA
		if len(lockedShort) > 7 {
			lockedShort = lockedShort[:7]
		}
		latestShort := o.LatestSHA
		if len(latestShort) > 7 {
			latestShort = latestShort[:7]
		}

		ref := o.Ref
		if len(ref) > 23 {
			ref = ref[:20] + "..."
		}

		age := formatDuration(o.Age)

		fmt.Printf("  %-10s │ %-25s │ %-10s │ %-10s │ %s\n",
			o.Type, ref, lockedShort, latestShort, age)
	}

	fmt.Println()
	fmt.Println("Update with: scm remote [bundles|profiles] pull <reference>")

	return nil
}

// getLatestSHA gets the latest commit SHA on the default branch.
func getLatestSHA(ctx context.Context, fetcher remote.Fetcher, owner, repo string) (string, error) {
	branch, err := fetcher.GetDefaultBranch(ctx, owner, repo)
	if err != nil {
		return "", err
	}
	return fetcher.ResolveRef(ctx, owner, repo, branch)
}

// formatDuration formats a duration in human-readable form.
func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	if days > 30 {
		return fmt.Sprintf("%d months", days/30)
	}
	if days > 0 {
		return fmt.Sprintf("%d days", days)
	}
	hours := int(d.Hours())
	if hours > 0 {
		return fmt.Sprintf("%d hours", hours)
	}
	return "< 1 hour"
}

func init() {
	remoteCmd.AddCommand(remoteLockCmd)
	remoteCmd.AddCommand(remoteInstallCmd)
	remoteCmd.AddCommand(remoteOutdatedCmd)

	remoteInstallCmd.Flags().BoolVarP(&pullForce, "force", "f", false,
		"Skip confirmation prompts")
}
