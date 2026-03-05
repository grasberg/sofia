// Sofia - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 Sofia contributors

//go:build darwin

package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func isComputerUseSupported() bool { return true }

func computerUsePlatformError() string { return "" }

// takeDesktopScreenshot captures the full desktop using macOS screencapture.
// Returns the path to the saved PNG file.
func takeDesktopScreenshot(screenshotDir string) (string, error) {
	ts := time.Now().Format("20060102-150405")
	path := filepath.Join(screenshotDir, fmt.Sprintf("screen-%s.png", ts))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// -x: no sound, -t png: PNG format
	cmd := exec.CommandContext(ctx, "screencapture", "-x", "-t", "png", path)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("screencapture failed: %v — %s", err, strings.TrimSpace(stderr.String()))
	}
	return path, nil
}

// executeDesktopAction executes a single computer action on macOS using osascript.
func executeDesktopAction(a *computerAction) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var script string
	switch strings.ToLower(a.Action) {
	case "click":
		script = fmt.Sprintf(`
tell application "System Events"
    click at {%d, %d}
end tell`, a.X, a.Y)
	case "right_click":
		script = fmt.Sprintf(`
tell application "System Events"
    set p to {%d, %d}
    do shell script "cliclick rc:" & item 1 of p & "," & item 2 of p
end tell`, a.X, a.Y)
	case "double_click":
		script = fmt.Sprintf(`
tell application "System Events"
    double click at {%d, %d}
end tell`, a.X, a.Y)
	case "type":
		// Escape backslashes and double-quotes for AppleScript string
		escaped := strings.ReplaceAll(a.Text, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		script = fmt.Sprintf(`
tell application "System Events"
    keystroke "%s"
end tell`, escaped)
	case "press":
		osascriptKey, modifiers := parseOSAKey(a.Key)
		if modifiers != "" {
			script = fmt.Sprintf(`
tell application "System Events"
    keystroke "%s" using {%s}
end tell`, osascriptKey, modifiers)
		} else {
			script = fmt.Sprintf(`
tell application "System Events"
    key code %s
end tell`, osascriptKey)
		}
	case "scroll":
		// Use scroll via AppleScript
		script = fmt.Sprintf(`
tell application "System Events"
    scroll area 1 by delta x %d delta y %d
end tell`, a.DX, a.DY)
	case "screenshot", "done":
		return nil
	default:
		return fmt.Errorf("unknown action: %q", a.Action)
	}

	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("osascript failed for action %q: %v — %s", a.Action, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// parseOSAKey maps common key names to AppleScript key codes or modifiers.
// Returns (keyCode string, modifiers string).
// For keystroke (single chars), returns (char, "") or (char, modifiers).
// For key codes (special keys), returns (code, "").
func parseOSAKey(key string) (string, string) {
	lower := strings.ToLower(key)

	// Handle modifier+key combos like "ctrl+c", "cmd+v"
	if strings.Contains(lower, "+") {
		parts := strings.Split(lower, "+")
		mainKey := parts[len(parts)-1]
		mods := parts[:len(parts)-1]

		var osaMods []string
		for _, m := range mods {
			switch m {
			case "ctrl", "control":
				osaMods = append(osaMods, "control down")
			case "cmd", "command":
				osaMods = append(osaMods, "command down")
			case "alt", "option":
				osaMods = append(osaMods, "option down")
			case "shift":
				osaMods = append(osaMods, "shift down")
			}
		}
		return mainKey, strings.Join(osaMods, ", ")
	}

	// Special key codes for non-printable keys
	// https://eastmanreference.com/complete-list-of-applescript-key-codes
	specialKeys := map[string]string{
		"return":    "36",
		"enter":     "36",
		"tab":       "48",
		"escape":    "53",
		"esc":       "53",
		"delete":    "51",
		"backspace": "51",
		"space":     "49",
		"up":        "126",
		"down":      "125",
		"left":      "123",
		"right":     "124",
		"home":      "115",
		"end":       "119",
		"pageup":    "116",
		"pagedown":  "121",
		"f1":        "122",
		"f2":        "120",
		"f3":        "99",
		"f4":        "118",
		"f5":        "96",
		"f6":        "97",
		"f7":        "98",
		"f8":        "100",
		"f9":        "101",
		"f10":       "109",
		"f11":       "103",
		"f12":       "111",
	}

	if code, ok := specialKeys[lower]; ok {
		return code, ""
	}

	// Treat as a plain keystroke character
	return key, ""
}
