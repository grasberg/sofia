package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ScreenshotTool takes a desktop screenshot and returns the path for vision analysis.
type ScreenshotTool struct {
	workspace string
}

func NewScreenshotTool(workspace string) *ScreenshotTool {
	return &ScreenshotTool{workspace: workspace}
}

func (t *ScreenshotTool) Name() string { return "screenshot" }

func (t *ScreenshotTool) Description() string {
	return "Take a screenshot of the desktop screen. Returns the file path of the saved screenshot image. " +
		"Use this with image_analyze to understand what's on screen. " +
		"Useful for debugging UI issues, reading error dialogs, or understanding the user's current context."
}

func (t *ScreenshotTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"filename": map[string]any{
				"type":        "string",
				"description": "Optional filename for the screenshot (default: screenshot-{timestamp}.png)",
			},
		},
		"required": []string{},
	}
}

func (t *ScreenshotTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	filename, _ := args["filename"].(string)
	if filename == "" {
		filename = fmt.Sprintf("screenshot-%s.png", time.Now().Format("20060102-150405"))
	}
	if !strings.HasSuffix(filename, ".png") {
		filename += ".png"
	}

	outPath := filepath.Join(t.workspace, filename)
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return ErrorResult("screenshot: failed to create directory: " + err.Error())
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "screencapture", "-x", outPath)
	case "linux":
		// Try gnome-screenshot, then scrot, then import (ImageMagick)
		if _, err := exec.LookPath("gnome-screenshot"); err == nil {
			cmd = exec.CommandContext(ctx, "gnome-screenshot", "-f", outPath)
		} else if _, err := exec.LookPath("scrot"); err == nil {
			cmd = exec.CommandContext(ctx, "scrot", outPath)
		} else if _, err := exec.LookPath("import"); err == nil {
			cmd = exec.CommandContext(ctx, "import", "-window", "root", outPath)
		} else {
			return ErrorResult("screenshot: no screenshot tool found. Install scrot or gnome-screenshot.")
		}
	case "windows":
		// Use PowerShell to take a screenshot.
		// Sanitize the output path to prevent command injection: use a fixed temp
		// filename, then rename after capture.
		tempName := fmt.Sprintf("sofia_screenshot_%d.png", time.Now().UnixNano())
		tempPath := filepath.Join(os.TempDir(), tempName)
		psCmd := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms; `+
			`$bmp = New-Object System.Drawing.Bitmap([System.Windows.Forms.Screen]::PrimaryScreen.Bounds.Width, [System.Windows.Forms.Screen]::PrimaryScreen.Bounds.Height); `+
			`$g = [System.Drawing.Graphics]::FromImage($bmp); `+
			`$g.CopyFromScreen(0, 0, 0, 0, $bmp.Size); `+
			`$bmp.Save('%s'); `+
			`$g.Dispose(); $bmp.Dispose()`, strings.ReplaceAll(tempPath, "'", "''"))
		cmd = exec.CommandContext(ctx, "powershell", "-Command", psCmd)
		// After running the command, rename temp file to the actual outPath.
		defer func() {
			if _, err := os.Stat(tempPath); err == nil {
				_ = os.Rename(tempPath, outPath)
			}
		}()
	default:
		return ErrorResult("screenshot: unsupported platform " + runtime.GOOS)
	}

	if err := cmd.Run(); err != nil {
		return ErrorResult("screenshot: capture failed: " + err.Error())
	}

	// Verify file was created
	info, err := os.Stat(outPath)
	if err != nil || info.Size() == 0 {
		return ErrorResult("screenshot: file was not created")
	}

	result := fmt.Sprintf(
		"Screenshot saved to: %s (%d KB)\nUse image_analyze tool to examine the screenshot content.",
		outPath,
		info.Size()/1024,
	)
	return &ToolResult{ForLLM: result, ForUser: result, IsError: false}
}
