// Sofia - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 Sofia contributors

//go:build !darwin && !linux

package tools

import "fmt"

func isComputerUseSupported() bool { return false }

func computerUsePlatformError() string {
	return "computer_use is only supported on macOS and Linux"
}

func takeDesktopScreenshot(_ string) (string, error) {
	return "", fmt.Errorf("computer_use is only supported on macOS and Linux")
}

func executeDesktopAction(_ *computerAction) error {
	return fmt.Errorf("computer_use is only supported on macOS and Linux")
}
