package remote

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestDisplaySecurityWarning(t *testing.T) {
	var buf bytes.Buffer

	ref := &Reference{
		Remote: "alice",
		Path:   "security",
		GitRef: "v1.0.0",
	}
	rem := &Remote{
		Name:    "alice",
		URL:     "https://github.com/alice/scm",
		Version: "v1",
	}
	sha := "abc1234"
	filePath := "scm/v1/bundles/security.yaml"
	content := []byte("description: Test bundle\nfragments:\n  tdd:\n    content: Test content here\n")

	secure, _ := ParseSecureContent(ItemTypeBundle, content)
	displaySecurityWarning(&buf, ref, rem, sha, filePath, content, secure)

	output := buf.String()

	// Check warning banner is present (bundles show "BUNDLE INSTALLATION")
	if !strings.Contains(output, "WARNING: BUNDLE INSTALLATION") {
		t.Error("Missing warning banner")
	}

	// Check source info
	if !strings.Contains(output, "https://github.com/alice/scm") {
		t.Error("Missing source URL")
	}
	if !strings.Contains(output, "abc1234") {
		t.Error("Missing SHA")
	}
	if !strings.Contains(output, "alice") {
		t.Error("Missing org")
	}
	if !strings.Contains(output, "security") {
		t.Error("Missing name")
	}

	// Check content markers
	if !strings.Contains(output, "CONTENT START") {
		t.Error("Missing content start marker")
	}
	if !strings.Contains(output, "CONTENT END") {
		t.Error("Missing content end marker")
	}

	// Check content is present
	if !strings.Contains(output, "Test content here") {
		t.Error("Missing content body")
	}
}

func TestDisplaySecurityWarningProfile(t *testing.T) {
	var buf bytes.Buffer

	ref := &Reference{
		Remote: "alice",
		Path:   "secure",
		GitRef: "v1.0.0",
	}
	rem := &Remote{
		Name:    "alice",
		URL:     "https://github.com/alice/scm",
		Version: "v1",
	}
	sha := "abc1234"
	filePath := "scm/v1/profiles/secure.yaml"
	content := []byte("name: secure\nbundles:\n  - alice/security\n")

	secure, _ := ParseSecureContent(ItemTypeProfile, content)
	displaySecurityWarning(&buf, ref, rem, sha, filePath, content, secure)

	output := buf.String()

	// Check warning banner is present
	if !strings.Contains(output, "WARNING: PROMPT INJECTION RISK") {
		t.Error("Missing warning banner")
	}

	// Check source info
	if !strings.Contains(output, "https://github.com/alice/scm") {
		t.Error("Missing source URL")
	}

	// Check content markers
	if !strings.Contains(output, "CONTENT START") {
		t.Error("Missing content start marker")
	}
}

func TestPromptConfirmation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"lowercase y", "y\n", true},
		{"uppercase Y", "Y\n", true},
		{"lowercase yes", "yes\n", true},
		{"uppercase YES", "YES\n", true},
		{"mixed case Yes", "Yes\n", true},
		{"n", "n\n", false},
		{"no", "no\n", false},
		{"empty", "\n", false},
		{"other text", "maybe\n", false},
		{"y with spaces", "  y  \n", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			reader := strings.NewReader(tt.input)

			got, err := promptConfirmation(&buf, reader, "Test prompt")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.expected {
				t.Errorf("promptConfirmation() = %v, want %v", got, tt.expected)
			}

			// Check prompt was written
			if !strings.Contains(buf.String(), "Test prompt") {
				t.Error("prompt not written to output")
			}
			if !strings.Contains(buf.String(), "[y/N]") {
				t.Error("default indicator not in prompt")
			}
		})
	}
}

// mockFetcher is a test double for Fetcher.
type mockFetcher struct {
	files         map[string][]byte
	defaultBranch string
	refs          map[string]string
	forge         ForgeType
}

func newMockFetcher() *mockFetcher {
	return &mockFetcher{
		files:         make(map[string][]byte),
		defaultBranch: "main",
		refs:          make(map[string]string),
		forge:         ForgeGitHub,
	}
}

func (m *mockFetcher) FetchFile(ctx context.Context, owner, repo, path, ref string) ([]byte, error) {
	if content, ok := m.files[path]; ok {
		return content, nil
	}
	return nil, &fileNotFoundError{path: path}
}

func (m *mockFetcher) ListDir(ctx context.Context, owner, repo, path, ref string) ([]DirEntry, error) {
	return nil, nil
}

func (m *mockFetcher) ResolveRef(ctx context.Context, owner, repo, ref string) (string, error) {
	if sha, ok := m.refs[ref]; ok {
		return sha, nil
	}
	// Default to returning the ref as-is for testing
	return ref + "000000", nil
}

func (m *mockFetcher) SearchRepos(ctx context.Context, query string, limit int) ([]RepoInfo, error) {
	return nil, nil
}

func (m *mockFetcher) ValidateRepo(ctx context.Context, owner, repo string) (bool, error) {
	return true, nil
}

func (m *mockFetcher) GetDefaultBranch(ctx context.Context, owner, repo string) (string, error) {
	return m.defaultBranch, nil
}

func (m *mockFetcher) Forge() ForgeType {
	return m.forge
}

type fileNotFoundError struct {
	path string
}

func (e *fileNotFoundError) Error() string {
	return "file not found: " + e.path
}
