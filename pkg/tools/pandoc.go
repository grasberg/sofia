package tools

import (
	"context"
	"fmt"
	"time"
)

// PandocTool wraps Pandoc for document format conversion.
type PandocTool struct {
	binaryPath string
	timeout    time.Duration
}

func NewPandocTool(binaryPath string, timeoutSeconds int) *PandocTool {
	if binaryPath == "" {
		binaryPath = "pandoc"
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 60
	}
	return &PandocTool{
		binaryPath: binaryPath,
		timeout:    time.Duration(timeoutSeconds) * time.Second,
	}
}

func (t *PandocTool) Name() string { return "pandoc" }
func (t *PandocTool) Description() string {
	return "Convert documents between formats using Pandoc. Supports: markdown, html, pdf, docx, latex, rst, org, epub, plain text, and many more. Requires Pandoc to be installed."
}

func (t *PandocTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input": map[string]any{
				"type":        "string",
				"description": "Input file path or '-' for stdin content",
			},
			"input_content": map[string]any{
				"type":        "string",
				"description": "Input content (alternative to file path). Used when input is '-'.",
			},
			"output": map[string]any{
				"type":        "string",
				"description": "Output file path. If omitted, output goes to stdout.",
			},
			"from": map[string]any{
				"type":        "string",
				"description": "Input format (e.g., markdown, html, latex, docx, rst, org). Auto-detected from extension if omitted.",
			},
			"to": map[string]any{
				"type":        "string",
				"description": "Output format (e.g., html, pdf, docx, latex, plain, rst, org, epub)",
			},
			"args": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
				"description": "Additional pandoc arguments (e.g., --toc, --standalone, --template)",
			},
		},
		"required": []string{"to"},
	}
}

func (t *PandocTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	toFormat, _ := args["to"].(string)
	if toFormat == "" {
		return ErrorResult("'to' format is required")
	}

	pandocArgs := []string{}

	if from, ok := args["from"].(string); ok && from != "" {
		pandocArgs = append(pandocArgs, "-f", from)
	}
	pandocArgs = append(pandocArgs, "-t", toFormat)

	if output, ok := args["output"].(string); ok && output != "" {
		pandocArgs = append(pandocArgs, "-o", output)
	}

	// Add extra args
	if raw, ok := args["args"]; ok {
		parsed, _ := parseStringArgs(raw)
		pandocArgs = append(pandocArgs, parsed...)
	}

	// Add input file
	if input, ok := args["input"].(string); ok && input != "" && input != "-" {
		pandocArgs = append(pandocArgs, input)
	}

	timeout := t.timeout
	if raw, ok := args["timeout_seconds"]; ok {
		if n, ok := parsePositiveInt(raw); ok {
			timeout = time.Duration(n) * time.Second
		}
	}

	result := ExecuteCLICommand(CLICommandInput{
		Ctx:         ctx,
		BinaryPath:  t.binaryPath,
		Args:        pandocArgs,
		Timeout:     timeout,
		ToolName:    "pandoc",
		InstallHint: "Install Pandoc: brew install pandoc",
	})

	if !result.IsError && result.ForLLM == "(no output)" {
		if output, ok := args["output"].(string); ok && output != "" {
			return SilentResult(fmt.Sprintf("Document converted and saved to: %s", output))
		}
	}

	return result
}
