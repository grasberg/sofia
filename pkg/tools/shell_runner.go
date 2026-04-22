package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
)

// runResult holds the outcome of a single command execution attempt.
type runResult struct {
	output   string
	timedOut bool
	err      error
}

// runOnce executes the command once with the given timeout and returns a runResult.
func (t *ExecTool) runOnce(ctx context.Context, command, cwd string, timeout time.Duration) runResult {
	var cmdCtx context.Context
	var cancel context.CancelFunc
	if timeout > 0 {
		cmdCtx, cancel = context.WithTimeout(ctx, timeout)
	} else {
		cmdCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	var cmd *exec.Cmd

	// Guardrail: Sandboxed Execution via Docker
	if t.sandboxConfig != nil && t.sandboxConfig.Enabled {
		image := t.sandboxConfig.DockerImage
		if image == "" {
			image = "alpine:latest" // Default
		}

		// Mount the workingDir as /workspace in the container
		mountDir := cwd
		if t.workingDir != "" && t.restrictToWorkspace {
			mountDir = t.workingDir // Always mount the root workspace if restricted
		}

		// Calculate relative path inside the container if we mounted a higher directory
		innerCwd := "/workspace"
		if mountDir != cwd && strings.HasPrefix(cwd, mountDir) {
			relPath, _ := filepath.Rel(mountDir, cwd)
			if relPath != "" && relPath != "." {
				innerCwd = filepath.Join(innerCwd, relPath)
			}
		}

		// docker run --rm --network none -v <mountDir>:/workspace -w <innerCwd> <image> sh -c <command>
		dockerArgs := []string{
			"run", "--rm",
			"--network", "none", // Prevent network access
			"-v", fmt.Sprintf("%s:/workspace", mountDir),
			"-w", innerCwd,
			image,
			"sh", "-c", command,
		}

		logger.Audit("Sandboxed Command Executed", map[string]any{
			"command": command,
			"image":   image,
			"cwd":     mountDir,
		})

		cmd = exec.CommandContext(cmdCtx, "docker", dockerArgs...)
	} else {
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(cmdCtx, "powershell", "-NoProfile", "-NonInteractive", "-Command", command)
		} else {
			cmd = exec.CommandContext(cmdCtx, "sh", "-c", command)
		}
		if cwd != "" {
			cmd.Dir = cwd
		}
		// Apply pre-computed enriched PATH so Homebrew, pyenv, etc. are found.
		if t.enrichedEnv != nil {
			cmd.Env = t.enrichedEnv
		}
	}

	prepareCommandForTermination(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return runResult{
			output: fmt.Sprintf("failed to start command: %v", err),
			err:    err,
		}
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	var err error
	select {
	case err = <-done:
	case <-cmdCtx.Done():
		_ = terminateProcessTree(cmd)
		select {
		case err = <-done:
		case <-time.After(2 * time.Second):
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			err = <-done
		}
	}

	if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
		return runResult{timedOut: true, err: err}
	}

	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\nSTDERR:\n" + stderr.String()
	}
	if err != nil {
		output += fmt.Sprintf("\nExit code: %v", err)
	}

	return runResult{output: output, err: err}
}
