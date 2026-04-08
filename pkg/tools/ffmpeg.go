package tools

import (
	"context"
	"fmt"
	"time"
)

// FFmpegTool wraps ffmpeg/ffprobe for media processing operations.
type FFmpegTool struct {
	binaryPath string
	timeout    time.Duration
}

func NewFFmpegTool(binaryPath string, timeoutSeconds int) *FFmpegTool {
	if binaryPath == "" {
		binaryPath = "ffmpeg"
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 300
	}
	return &FFmpegTool{
		binaryPath: binaryPath,
		timeout:    time.Duration(timeoutSeconds) * time.Second,
	}
}

func (t *FFmpegTool) Name() string { return "ffmpeg" }
func (t *FFmpegTool) Description() string {
	return "Process media files with ffmpeg. Actions: info (get media info via ffprobe), convert (format conversion), extract_frame (get a frame as image), trim (cut a section), compress (reduce size), audio_extract (extract audio track)."
}

func (t *FFmpegTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type": "string",
				"enum": []string{
					"info",
					"convert",
					"extract_frame",
					"trim",
					"compress",
					"audio_extract",
					"custom",
				},
				"description": "Action to perform",
			},
			"input": map[string]any{
				"type":        "string",
				"description": "Input file path",
			},
			"output": map[string]any{
				"type":        "string",
				"description": "Output file path (required for convert, extract_frame, trim, compress, audio_extract)",
			},
			"args": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
				"description": "Additional ffmpeg arguments (for custom action or overrides)",
			},
			"start_time": map[string]any{
				"type":        "string",
				"description": "Start time for trim/extract_frame (HH:MM:SS or seconds)",
			},
			"duration": map[string]any{
				"type":        "string",
				"description": "Duration for trim (HH:MM:SS or seconds)",
			},
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"description": "Command timeout (default 300)",
			},
		},
		"required": []string{"action", "input"},
	}
}

func (t *FFmpegTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	action, _ := args["action"].(string)
	input, _ := args["input"].(string)
	output, _ := args["output"].(string)

	if action == "" || input == "" {
		return ErrorResult("action and input are required")
	}

	timeout := t.timeout
	if raw, ok := args["timeout_seconds"]; ok {
		if n, ok := parsePositiveInt(raw); ok {
			timeout = time.Duration(n) * time.Second
		}
	}

	switch action {
	case "info":
		return t.getInfo(ctx, input, timeout)
	case "convert":
		if output == "" {
			return ErrorResult("output is required for convert")
		}
		return t.convert(ctx, input, output, args, timeout)
	case "extract_frame":
		if output == "" {
			return ErrorResult("output is required for extract_frame")
		}
		return t.extractFrame(ctx, input, output, args, timeout)
	case "trim":
		if output == "" {
			return ErrorResult("output is required for trim")
		}
		return t.trim(ctx, input, output, args, timeout)
	case "compress":
		if output == "" {
			return ErrorResult("output is required for compress")
		}
		return t.compress(ctx, input, output, timeout)
	case "audio_extract":
		if output == "" {
			return ErrorResult("output is required for audio_extract")
		}
		return t.audioExtract(ctx, input, output, timeout)
	case "custom":
		return t.custom(ctx, args, timeout)
	default:
		return ErrorResult(fmt.Sprintf("unknown action: %s", action))
	}
}

func (t *FFmpegTool) getInfo(ctx context.Context, input string, timeout time.Duration) *ToolResult {
	return ExecuteCLICommand(CLICommandInput{
		Ctx:         ctx,
		BinaryPath:  "ffprobe",
		Args:        []string{"-v", "quiet", "-print_format", "json", "-show_format", "-show_streams", input},
		Timeout:     timeout,
		ToolName:    "ffprobe",
		InstallHint: "Install ffmpeg: brew install ffmpeg",
	})
}

func (t *FFmpegTool) convert(
	ctx context.Context,
	input, output string,
	args map[string]any,
	timeout time.Duration,
) *ToolResult {
	ffArgs := []string{"-i", input, "-y"}
	if raw, ok := args["args"]; ok {
		parsed, _ := parseStringArgs(raw)
		ffArgs = append(ffArgs, parsed...)
	}
	ffArgs = append(ffArgs, output)

	return ExecuteCLICommand(CLICommandInput{
		Ctx:         ctx,
		BinaryPath:  t.binaryPath,
		Args:        ffArgs,
		Timeout:     timeout,
		ToolName:    "ffmpeg",
		InstallHint: "Install ffmpeg: brew install ffmpeg",
	})
}

func (t *FFmpegTool) extractFrame(
	ctx context.Context,
	input, output string,
	args map[string]any,
	timeout time.Duration,
) *ToolResult {
	startTime := "00:00:01"
	if st, ok := args["start_time"].(string); ok && st != "" {
		startTime = st
	}

	ffArgs := []string{"-i", input, "-ss", startTime, "-vframes", "1", "-y", output}

	return ExecuteCLICommand(CLICommandInput{
		Ctx:         ctx,
		BinaryPath:  t.binaryPath,
		Args:        ffArgs,
		Timeout:     timeout,
		ToolName:    "ffmpeg",
		InstallHint: "Install ffmpeg: brew install ffmpeg",
	})
}

func (t *FFmpegTool) trim(
	ctx context.Context,
	input, output string,
	args map[string]any,
	timeout time.Duration,
) *ToolResult {
	ffArgs := []string{"-i", input}

	if st, ok := args["start_time"].(string); ok && st != "" {
		ffArgs = append(ffArgs, "-ss", st)
	}
	if dur, ok := args["duration"].(string); ok && dur != "" {
		ffArgs = append(ffArgs, "-t", dur)
	}

	ffArgs = append(ffArgs, "-c", "copy", "-y", output)

	return ExecuteCLICommand(CLICommandInput{
		Ctx:         ctx,
		BinaryPath:  t.binaryPath,
		Args:        ffArgs,
		Timeout:     timeout,
		ToolName:    "ffmpeg",
		InstallHint: "Install ffmpeg: brew install ffmpeg",
	})
}

func (t *FFmpegTool) compress(ctx context.Context, input, output string, timeout time.Duration) *ToolResult {
	ffArgs := []string{"-i", input, "-crf", "28", "-preset", "medium", "-y", output}

	return ExecuteCLICommand(CLICommandInput{
		Ctx:         ctx,
		BinaryPath:  t.binaryPath,
		Args:        ffArgs,
		Timeout:     timeout,
		ToolName:    "ffmpeg",
		InstallHint: "Install ffmpeg: brew install ffmpeg",
	})
}

func (t *FFmpegTool) audioExtract(ctx context.Context, input, output string, timeout time.Duration) *ToolResult {
	ffArgs := []string{"-i", input, "-vn", "-acodec", "copy", "-y", output}

	return ExecuteCLICommand(CLICommandInput{
		Ctx:         ctx,
		BinaryPath:  t.binaryPath,
		Args:        ffArgs,
		Timeout:     timeout,
		ToolName:    "ffmpeg",
		InstallHint: "Install ffmpeg: brew install ffmpeg",
	})
}

func (t *FFmpegTool) custom(ctx context.Context, args map[string]any, timeout time.Duration) *ToolResult {
	raw, ok := args["args"]
	if !ok {
		return ErrorResult("args is required for custom action")
	}
	ffArgs, err := parseStringArgs(raw)
	if err != nil {
		return ErrorResult(err.Error())
	}

	return ExecuteCLICommand(CLICommandInput{
		Ctx:         ctx,
		BinaryPath:  t.binaryPath,
		Args:        ffArgs,
		Timeout:     timeout,
		ToolName:    "ffmpeg",
		InstallHint: "Install ffmpeg: brew install ffmpeg",
	})
}
