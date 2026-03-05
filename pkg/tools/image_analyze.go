// Sofia - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 Sofia contributors

package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultMaxImageSizeKB = 4096
	maxImageReadBytes     = defaultMaxImageSizeKB * 1024
)

// ImageAnalyzeTool reads a local image file and injects it into the LLM context
// via the vision pipeline that is already wired in Sofia's providers.
// The LLM will receive the image and any optional question in the next turn.
type ImageAnalyzeTool struct {
	workspace string
	restrict  bool
	maxSizeKB int
}

// NewImageAnalyzeTool creates an ImageAnalyzeTool.
// workspace is used to resolve relative paths; restrict gates access to workspace-only paths.
func NewImageAnalyzeTool(workspace string, restrict bool) *ImageAnalyzeTool {
	return &ImageAnalyzeTool{
		workspace: workspace,
		restrict:  restrict,
		maxSizeKB: defaultMaxImageSizeKB,
	}
}

func (t *ImageAnalyzeTool) Name() string { return "image_analyze" }

func (t *ImageAnalyzeTool) Description() string {
	return "Read a local image file (PNG, JPEG, GIF, WebP) and send it to the vision-capable LLM " +
		"for analysis. Use this to describe, OCR, or answer questions about screenshots or photos."
}

func (t *ImageAnalyzeTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{
				"type": "string",
				"description": "Path to the image file to analyze (PNG, JPEG, GIF, WebP). " +
					"May be absolute or relative to the workspace.",
			},
			"question": map[string]any{
				"type": "string",
				"description": "Optional question or instruction about the image, e.g. " +
					"\"What text is visible?\", \"Describe this screenshot.\", \"What is the error message?\"",
			},
		},
		"required": []string{"file_path"},
	}
}

func (t *ImageAnalyzeTool) Execute(_ context.Context, args map[string]any) *ToolResult {
	filePath, ok := args["file_path"].(string)
	if !ok || strings.TrimSpace(filePath) == "" {
		return ErrorResult("file_path is required")
	}
	filePath = strings.TrimSpace(filePath)

	question := ""
	if q, ok := args["question"].(string); ok {
		question = strings.TrimSpace(q)
	}

	// Resolve path: if relative, anchor to workspace
	resolved := filePath
	if !filepath.IsAbs(filePath) {
		resolved = filepath.Join(t.workspace, filePath)
	}
	resolved = filepath.Clean(resolved)

	// Workspace restriction
	if t.restrict {
		cleanWorkspace := filepath.Clean(t.workspace)
		if !strings.HasPrefix(resolved, cleanWorkspace+string(os.PathSeparator)) && resolved != cleanWorkspace {
			return ErrorResult(fmt.Sprintf("access denied: %q is outside the workspace", filePath))
		}
	}

	// Read file
	info, err := os.Stat(resolved)
	if err != nil {
		return ErrorResult(fmt.Sprintf("cannot access file %q: %v", filePath, err))
	}
	if info.IsDir() {
		return ErrorResult(fmt.Sprintf("%q is a directory, not an image file", filePath))
	}
	sizeKB := info.Size() / 1024
	if sizeKB > int64(t.maxSizeKB) {
		return ErrorResult(fmt.Sprintf(
			"image file is too large (%d KB). Maximum allowed size is %d KB.",
			sizeKB, t.maxSizeKB,
		))
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to read image file %q: %v", filePath, err))
	}

	// Detect MIME type
	mimeType := detectImageMIME(resolved, data)
	if mimeType == "" {
		return ErrorResult(fmt.Sprintf(
			"unsupported image format for %q. Supported formats: PNG, JPEG, GIF, WebP",
			filePath,
		))
	}

	// Build base64 data URL
	encoded := base64.StdEncoding.EncodeToString(data)
	dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, encoded)

	// Build the LLM prompt
	prompt := question
	if prompt == "" {
		prompt = "Describe this image in detail."
	}
	forLLM := fmt.Sprintf("%s\n[image attached: %s (%d KB)]", prompt, filepath.Base(resolved), sizeKB)

	return &ToolResult{
		ForLLM:  forLLM,
		ForUser: fmt.Sprintf("Image loaded: %s (%d KB, %s)", filepath.Base(resolved), sizeKB, mimeType),
		Images:  []string{dataURL},
	}
}

// detectImageMIME returns the MIME type for a supported image file.
// It uses the file extension first, then falls back to content sniffing.
// Returns "" for unsupported types.
func detectImageMIME(path string, data []byte) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	}

	// Fall back to content sniffing for files without a recognized extension
	if len(data) >= 512 {
		sniffed := http.DetectContentType(data[:512])
		switch {
		case strings.HasPrefix(sniffed, "image/png"):
			return "image/png"
		case strings.HasPrefix(sniffed, "image/jpeg"):
			return "image/jpeg"
		case strings.HasPrefix(sniffed, "image/gif"):
			return "image/gif"
		case strings.HasPrefix(sniffed, "image/webp"):
			return "image/webp"
		}
	}

	return ""
}
