package remote

import (
	"context"
	"testing"
)

func TestMockFetcher_SearchRepos(t *testing.T) {
	fetcher := newMockFetcher()

	// Test search returns nil for mock
	repos, err := fetcher.SearchRepos(context.Background(), "test", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repos != nil {
		t.Errorf("expected nil repos for mock, got %v", repos)
	}
}

func TestRepoInfo_Fields(t *testing.T) {
	repo := RepoInfo{
		Owner:       "alice",
		Name:        "scm",
		Description: "Test repo",
		Stars:       42,
		URL:         "https://github.com/alice/scm",
		Topics:      []string{"golang", "security"},
		Language:    "Go",
		Forge:       ForgeGitHub,
	}

	if repo.Owner != "alice" {
		t.Errorf("Owner = %q, want %q", repo.Owner, "alice")
	}
	if repo.Name != "scm" {
		t.Errorf("Name = %q, want %q", repo.Name, "scm")
	}
	if repo.Stars != 42 {
		t.Errorf("Stars = %d, want %d", repo.Stars, 42)
	}
	if repo.Forge != ForgeGitHub {
		t.Errorf("Forge = %q, want %q", repo.Forge, ForgeGitHub)
	}
	if len(repo.Topics) != 2 {
		t.Errorf("Topics length = %d, want %d", len(repo.Topics), 2)
	}
}

func TestForgeType_Values(t *testing.T) {
	tests := []struct {
		name  string
		forge ForgeType
		want  string
	}{
		{
			name:  "github",
			forge: ForgeGitHub,
			want:  "github",
		},
		{
			name:  "gitlab",
			forge: ForgeGitLab,
			want:  "gitlab",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(tt.forge); got != tt.want {
				t.Errorf("ForgeType = %q, want %q", got, tt.want)
			}
		})
	}
}
