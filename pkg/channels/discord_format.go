package channels

import (
	"fmt"
	"regexp"
	"strings"
)

// reDiscordCodeBlock matches fenced code blocks preserving the entire block verbatim,
// including the language hint (e.g. ```go ... ```). This differs from reCodeBlock used
// for Telegram which strips the language specifier.
var reDiscordCodeBlock = regexp.MustCompile("(?s)```[\\w]*\\n?[\\s\\S]*?```")

// reDiscordHeading matches markdown headings with multi-line mode so it works on
// lines within a larger text block, not just the full-string boundary.
var reDiscordHeading = regexp.MustCompile(`(?m)^#{1,6}\s+(.+)$`)

// formatDiscordMarkdown converts standard markdown to Discord-compatible markdown.
// Discord natively supports: **bold**, *italic*, ~~strikethrough~~, `code`,
// ```code blocks```, > blockquotes, and ||spoiler||.
// The main conversions needed are for headings (not natively rendered) and links.
func formatDiscordMarkdown(text string) string {
	if text == "" {
		return ""
	}

	// Extract code blocks verbatim (preserving language hints for syntax highlighting)
	// to protect them from heading/link transformations.
	var codeBlocksFull []string
	cbIdx := 0
	text = reDiscordCodeBlock.ReplaceAllStringFunc(text, func(m string) string {
		codeBlocksFull = append(codeBlocksFull, m)
		placeholder := fmt.Sprintf("\x00DCB%d\x00", cbIdx)
		cbIdx++
		return placeholder
	})

	// Extract inline codes to protect them as well.
	inlineCodes := extractInlineCodes(text)
	text = inlineCodes.text

	// Convert markdown headings to bold text (Discord doesn't render # headings).
	// Uses reDiscordHeading with (?m) multi-line mode for proper line matching.
	text = reDiscordHeading.ReplaceAllString(text, "**$1**")

	// Convert markdown links [text](url) to Discord-friendly format.
	// Discord supports masked links in embeds but in regular messages [text](url) renders
	// poorly, so convert to: **text** (<url>)
	text = reLink.ReplaceAllStringFunc(text, func(s string) string {
		match := reLink.FindStringSubmatch(s)
		if len(match) < 3 {
			return s
		}
		linkText := match[1]
		linkURL := match[2]
		if linkText == linkURL {
			return linkURL
		}
		return fmt.Sprintf("**%s** (<%s>)", linkText, linkURL)
	})

	// Restore inline codes
	for i, code := range inlineCodes.codes {
		text = strings.ReplaceAll(text, fmt.Sprintf("\x00IC%d\x00", i), fmt.Sprintf("`%s`", code))
	}

	// Restore code blocks verbatim
	for i, block := range codeBlocksFull {
		text = strings.ReplaceAll(text, fmt.Sprintf("\x00DCB%d\x00", i), block)
	}

	return text
}
