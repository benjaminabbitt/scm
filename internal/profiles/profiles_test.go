package profiles

import (
	"testing"
)

func TestParseContentRef(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected ContentRef
	}{
		{
			name:  "simple bundle",
			input: "go-development",
			expected: ContentRef{
				Raw:    "go-development",
				Bundle: "go-development",
			},
		},
		{
			name:  "bundle with fragment",
			input: "go-development#fragments/testing",
			expected: ContentRef{
				Raw:      "go-development#fragments/testing",
				Bundle:   "go-development",
				ItemType: "fragments",
				ItemName: "testing",
			},
		},
		{
			name:  "bundle with prompt",
			input: "go-development#prompts/review",
			expected: ContentRef{
				Raw:      "go-development#prompts/review",
				Bundle:   "go-development",
				ItemType: "prompts",
				ItemName: "review",
			},
		},
		{
			name:  "bundle with mcp",
			input: "go-development#mcp/tasks",
			expected: ContentRef{
				Raw:      "go-development#mcp/tasks",
				Bundle:   "go-development",
				ItemType: "mcp",
				ItemName: "tasks",
			},
		},
		{
			name:  "remote/bundle",
			input: "github/go-development",
			expected: ContentRef{
				Raw:    "github/go-development",
				Remote: "github",
				Bundle: "go-development",
			},
		},
		{
			name:  "remote/bundle with fragment",
			input: "github/go-development#fragments/testing",
			expected: ContentRef{
				Raw:      "github/go-development#fragments/testing",
				Remote:   "github",
				Bundle:   "go-development",
				ItemType: "fragments",
				ItemName: "testing",
			},
		},
		{
			name:  "https URL",
			input: "https://github.com/user/scm-github",
			expected: ContentRef{
				Raw:    "https://github.com/user/scm-github",
				Remote: "https://github.com/user/scm-github",
				Bundle: "scm-github",
				IsURL:  true,
			},
		},
		{
			name:  "https URL with .git",
			input: "https://github.com/user/scm-github.git",
			expected: ContentRef{
				Raw:    "https://github.com/user/scm-github.git",
				Remote: "https://github.com/user/scm-github.git",
				Bundle: "scm-github",
				IsURL:  true,
			},
		},
		{
			name:  "https URL with fragment",
			input: "https://github.com/user/scm-github#fragments/testing",
			expected: ContentRef{
				Raw:      "https://github.com/user/scm-github#fragments/testing",
				Remote:   "https://github.com/user/scm-github",
				Bundle:   "scm-github",
				ItemType: "fragments",
				ItemName: "testing",
				IsURL:    true,
			},
		},
		{
			name:  "git@ SSH URL",
			input: "git@github.com:user/scm-github",
			expected: ContentRef{
				Raw:    "git@github.com:user/scm-github",
				Remote: "git@github.com:user/scm-github",
				Bundle: "scm-github",
				IsURL:  true,
			},
		},
		{
			name:  "git@ SSH URL with .git",
			input: "git@github.com:user/scm-github.git",
			expected: ContentRef{
				Raw:    "git@github.com:user/scm-github.git",
				Remote: "git@github.com:user/scm-github.git",
				Bundle: "scm-github",
				IsURL:  true,
			},
		},
		{
			name:  "git@ SSH URL with fragment",
			input: "git@github.com:user/scm-github#fragments/testing",
			expected: ContentRef{
				Raw:      "git@github.com:user/scm-github#fragments/testing",
				Remote:   "git@github.com:user/scm-github",
				Bundle:   "scm-github",
				ItemType: "fragments",
				ItemName: "testing",
				IsURL:    true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseContentRef(tt.input)

			if got.Raw != tt.expected.Raw {
				t.Errorf("Raw: got %q, want %q", got.Raw, tt.expected.Raw)
			}
			if got.Remote != tt.expected.Remote {
				t.Errorf("Remote: got %q, want %q", got.Remote, tt.expected.Remote)
			}
			if got.Bundle != tt.expected.Bundle {
				t.Errorf("Bundle: got %q, want %q", got.Bundle, tt.expected.Bundle)
			}
			if got.ItemType != tt.expected.ItemType {
				t.Errorf("ItemType: got %q, want %q", got.ItemType, tt.expected.ItemType)
			}
			if got.ItemName != tt.expected.ItemName {
				t.Errorf("ItemName: got %q, want %q", got.ItemName, tt.expected.ItemName)
			}
			if got.IsURL != tt.expected.IsURL {
				t.Errorf("IsURL: got %v, want %v", got.IsURL, tt.expected.IsURL)
			}
		})
	}
}

func TestContentRefMethods(t *testing.T) {
	tests := []struct {
		input      string
		isBundle   bool
		isFragment bool
		isPrompt   bool
		isMCP      bool
		bundlePath string
	}{
		{"go-dev", true, false, false, false, "go-dev"},
		{"go-dev#fragments/test", false, true, false, false, "go-dev"},
		{"go-dev#prompts/review", false, false, true, false, "go-dev"},
		{"go-dev#mcp/server", false, false, false, true, "go-dev"},
		{"github/go-dev", true, false, false, false, "github/go-dev"},
		{"github/go-dev#fragments/test", false, true, false, false, "github/go-dev"},
		{"https://github.com/user/repo#mcp/server", false, false, false, true, "https://github.com/user/repo"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			ref := ParseContentRef(tt.input)

			if ref.IsBundle() != tt.isBundle {
				t.Errorf("IsBundle: got %v, want %v", ref.IsBundle(), tt.isBundle)
			}
			if ref.IsFragment() != tt.isFragment {
				t.Errorf("IsFragment: got %v, want %v", ref.IsFragment(), tt.isFragment)
			}
			if ref.IsPrompt() != tt.isPrompt {
				t.Errorf("IsPrompt: got %v, want %v", ref.IsPrompt(), tt.isPrompt)
			}
			if ref.IsMCP() != tt.isMCP {
				t.Errorf("IsMCP: got %v, want %v", ref.IsMCP(), tt.isMCP)
			}
			if ref.BundlePath() != tt.bundlePath {
				t.Errorf("BundlePath: got %q, want %q", ref.BundlePath(), tt.bundlePath)
			}
		})
	}
}
