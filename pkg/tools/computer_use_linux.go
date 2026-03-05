// Sofia - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 Sofia contributors

//go:build linux

package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func isComputerUseSupported() bool { return true }

func computerUsePlatformError() string { return "" }

// takeDesktopScreenshot captures the full desktop on Linux.
// Tries scrot first, then gnome-screenshot, then import (ImageMagick).
func takeDesktopScreenshot(screenshotDir string) (string, error) {
	ts := time.Now().Format("20060102-150405")
	path := filepath.Join(screenshotDir, fmt.Sprintf("screen-%s.png", ts))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Try scrot (lightweight, widely available)
	if _, err := exec.LookPath("scrot"); err == nil {
		cmd := exec.CommandContext(ctx, "scrot", path)
		setDisplayEnv(cmd)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err == nil {
			return path, nil
		}
	}

	// Try gnome-screenshot
	if _, err := exec.LookPath("gnome-screenshot"); err == nil {
		cmd := exec.CommandContext(ctx, "gnome-screenshot", "-f", path)
		setDisplayEnv(cmd)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err == nil {
			return path, nil
		}
	}

	// Try ImageMagick import
	if _, err := exec.LookPath("import"); err == nil {
		cmd := exec.CommandContext(ctx, "import", "-window", "root", path)
		setDisplayEnv(cmd)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf(
		"no screenshot tool found; install one of: scrot, gnome-screenshot, or imagemagick (import)",
	)
}

// executeDesktopAction executes a single computer action on Linux using xdotool.
func executeDesktopAction(a *computerAction) error {
	if _, err := exec.LookPath("xdotool"); err != nil {
		return fmt.Errorf(
			"xdotool not found; install it with: sudo apt-get install xdotool (or equivalent)",
		)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	switch strings.ToLower(a.Action) {
	case "click":
		cmd = exec.CommandContext(ctx, "xdotool", "mousemove", "--sync",
			fmt.Sprint(a.X), fmt.Sprint(a.Y), "click", "1")
	case "right_click":
		cmd = exec.CommandContext(ctx, "xdotool", "mousemove", "--sync",
			fmt.Sprint(a.X), fmt.Sprint(a.Y), "click", "3")
	case "double_click":
		cmd = exec.CommandContext(ctx, "xdotool", "mousemove", "--sync",
			fmt.Sprint(a.X), fmt.Sprint(a.Y), "click", "--repeat", "2", "--delay", "100", "1")
	case "type":
		cmd = exec.CommandContext(ctx, "xdotool", "type", "--clearmodifiers", "--delay", "50", "--", a.Text)
	case "press":
		xdoKey := normalizeXdotoolKey(a.Key)
		cmd = exec.CommandContext(ctx, "xdotool", "key", "--clearmodifiers", xdoKey)
	case "scroll":
		// xdotool scroll: button 4 = up, button 5 = down
		button := "5"
		repeats := a.DY
		if a.DY < 0 {
			button = "4"
			repeats = -a.DY
		}
		if repeats == 0 {
			repeats = 3
		}
		cmd = exec.CommandContext(ctx, "xdotool", "mousemove", fmt.Sprint(a.X), fmt.Sprint(a.Y),
			"click", "--repeat", fmt.Sprint(repeats), button)
	case "screenshot", "done":
		return nil
	default:
		return fmt.Errorf("unknown action: %q", a.Action)
	}

	setDisplayEnv(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("xdotool failed for action %q: %v — %s", a.Action, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// normalizeXdotoolKey maps common key names to xdotool key names.
func normalizeXdotoolKey(key string) string {
	// xdotool uses X11 key names with + for modifiers (e.g. ctrl+c, Return, Tab)
	lower := strings.ToLower(key)

	replacements := map[string]string{
		"enter":     "Return",
		"return":    "Return",
		"escape":    "Escape",
		"esc":       "Escape",
		"delete":    "Delete",
		"del":       "Delete",
		"backspace": "BackSpace",
		"tab":       "Tab",
		"space":     "space",
		"up":        "Up",
		"down":      "Down",
		"left":      "Left",
		"right":     "Right",
		"home":      "Home",
		"end":       "End",
		"pageup":    "Prior",
		"pagedown":  "Next",
	}

	if mapped, ok := replacements[lower]; ok {
		return mapped
	}

	// Handle modifier combos: normalize ctrl+c → ctrl+c (xdotool format)
	key = strings.ReplaceAll(key, "cmd", "super")
	key = strings.ReplaceAll(key, "command", "super")
	key = strings.ReplaceAll(key, "alt", "alt")
	return key
}

// setDisplayEnv ensures DISPLAY is set for headless-friendly execution.
func setDisplayEnv(cmd *exec.Cmd) {
	display := os.Getenv("DISPLAY")
	if display == "" {
		display = ":0"
	}
	// Preserve existing env, add/override DISPLAY
	env := os.Environ()
	found := false
	for i, e := range env {
		if strings.HasPrefix(e, "DISPLAY=") {
			env[i] = "DISPLAY=" + display
			found = true
			break
		}
	}
	if !found {
		env = append(env, "DISPLAY="+display)
	}
	cmd.Env = env
}
