package channels

import "testing"

func TestFormatDiscordMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "plain text unchanged",
			input:    "Hello world",
			expected: "Hello world",
		},
		{
			name:     "bold preserved",
			input:    "This is **bold** text",
			expected: "This is **bold** text",
		},
		{
			name:     "italic preserved",
			input:    "This is *italic* text",
			expected: "This is *italic* text",
		},
		{
			name:     "strikethrough preserved",
			input:    "This is ~~deleted~~ text",
			expected: "This is ~~deleted~~ text",
		},
		{
			name:     "inline code preserved",
			input:    "Use `fmt.Println` to print",
			expected: "Use `fmt.Println` to print",
		},
		{
			name:     "heading converted to bold",
			input:    "## My Heading",
			expected: "**My Heading**",
		},
		{
			name:     "h1 heading converted to bold",
			input:    "# Title",
			expected: "**Title**",
		},
		{
			name:     "link converted to bold text with angle-bracket URL",
			input:    "Check [Google](https://google.com) for info",
			expected: "Check **Google** (<https://google.com>) for info",
		},
		{
			name:     "link with same text and URL shows just URL",
			input:    "[https://example.com](https://example.com)",
			expected: "https://example.com",
		},
		{
			name:     "code block with language hint preserved",
			input:    "```go\nfmt.Println(\"hello\")\n```",
			expected: "```go\nfmt.Println(\"hello\")\n```",
		},
		{
			name:     "code block without language preserved",
			input:    "```\nsome code\n```",
			expected: "```\nsome code\n```",
		},
		{
			name:     "heading inside code block not converted",
			input:    "```\n# comment\n```",
			expected: "```\n# comment\n```",
		},
		{
			name:     "link inside code block not converted",
			input:    "before\n```\n[text](url)\n```\nafter",
			expected: "before\n```\n[text](url)\n```\nafter",
		},
		{
			name:     "inline code protects heading-like content",
			input:    "`# not a heading`",
			expected: "`# not a heading`",
		},
		{
			name:     "blockquote preserved",
			input:    "> This is a quote",
			expected: "> This is a quote",
		},
		{
			name:     "mixed content",
			input:    "## Results\n\nHere is the **output**:\n\n```python\nprint('hello')\n```\n\nSee [docs](https://docs.example.com) for more.",
			expected: "**Results**\n\nHere is the **output**:\n\n```python\nprint('hello')\n```\n\nSee **docs** (<https://docs.example.com>) for more.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDiscordMarkdown(tt.input)
			if got != tt.expected {
				t.Errorf("formatDiscordMarkdown() =\n%q\nwant:\n%q", got, tt.expected)
			}
		})
	}
}
