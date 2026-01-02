package cmd

import "testing"

func TestStripDistillPreamble(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no preamble",
			input:    "# Title\n\nContent here",
			expected: "# Title\n\nContent here",
		},
		{
			name:     "here's prefix",
			input:    "Here's the compressed version:\n\n# Title\n\nContent",
			expected: "# Title\n\nContent",
		},
		{
			name:     "here is prefix",
			input:    "Here is my compressed version:\n# Title",
			expected: "# Title",
		},
		{
			name:     "my compressed version prefix",
			input:    "My compressed version of the content:\n\n# Title",
			expected: "# Title",
		},
		{
			name:     "separator with preamble",
			input:    "Here's the result:\n\n---\n\n# Title\n\nContent",
			expected: "# Title\n\nContent",
		},
		{
			name:     "code fence yaml",
			input:    "```yaml\n# Title\n\nContent\n```",
			expected: "# Title\n\nContent",
		},
		{
			name:     "code fence markdown",
			input:    "```markdown\n# Title\n\nContent\n```",
			expected: "# Title\n\nContent",
		},
		{
			name:     "code fence with preamble",
			input:    "Here's the output:\n```yaml\n# Title\n```",
			expected: "# Title",
		},
		{
			name:     "preserve mid-content separator",
			input:    "# Title\n\nSection 1\n\n---\n\nSection 2",
			expected: "# Title\n\nSection 1\n\n---\n\nSection 2",
		},
		{
			name:     "case insensitive prefix",
			input:    "HERE'S the compressed version:\n\n# Title",
			expected: "# Title",
		},
		{
			name:     "below is prefix",
			input:    "Below is the compressed content:\n\n# Title",
			expected: "# Title",
		},
		{
			name:     "i've compressed prefix",
			input:    "I've compressed the content:\n# Result",
			expected: "# Result",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripDistillPreamble(tt.input)
			if got != tt.expected {
				t.Errorf("stripDistillPreamble() =\n%q\nwant:\n%q", got, tt.expected)
			}
		})
	}
}
