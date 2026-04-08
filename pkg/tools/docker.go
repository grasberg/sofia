package tools

import (
	"context"
	"fmt"
	"time"
)

// DockerTool wraps the Docker CLI for container management operations.
type DockerTool struct {
	binaryPath string
	timeout    time.Duration
}

func NewDockerTool(binaryPath string, timeoutSeconds int) *DockerTool {
	if binaryPath == "" {
		binaryPath = "docker"
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 120
	}
	return &DockerTool{
		binaryPath: binaryPath,
		timeout:    time.Duration(timeoutSeconds) * time.Second,
	}
}

func (t *DockerTool) Name() string { return "docker" }
func (t *DockerTool) Description() string {
	return "Manage Docker containers and images. Commands: ps, images, run, exec, logs, stop, rm, build, pull, inspect, compose. Requires Docker to be installed."
}

func (t *DockerTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "Docker command to run",
				"enum": []string{
					"ps",
					"images",
					"run",
					"exec",
					"logs",
					"stop",
					"rm",
					"build",
					"pull",
					"inspect",
					"network",
					"volume",
					"compose",
					"stats",
					"top",
				},
			},
			"args": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
				"description": "Additional arguments for the docker command",
			},
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"description": "Command timeout in seconds (default 120)",
			},
		},
		"required": []string{"command"},
	}
}

var dockerBlockedCommands = map[string]bool{
	"login":  true,
	"logout": true,
	"push":   true,
	"secret": true,
	"swarm":  true,
	"plugin": true,
}

func (t *DockerTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	command, _ := args["command"].(string)
	if command == "" {
		return ErrorResult("command is required")
	}

	if dockerBlockedCommands[command] {
		return ErrorResult(fmt.Sprintf("command %q is blocked for security reasons", command))
	}

	var cmdArgs []string
	if raw, ok := args["args"]; ok {
		parsed, err := parseStringArgs(raw)
		if err == nil {
			cmdArgs = parsed
		}
	}

	// Check for dangerous flags
	for _, arg := range cmdArgs {
		if arg == "--privileged" || arg == "--cap-add=ALL" {
			return ErrorResult(fmt.Sprintf("flag %q is blocked for security", arg))
		}
	}

	timeout := t.timeout
	if raw, ok := args["timeout_seconds"]; ok {
		if n, ok := parsePositiveInt(raw); ok {
			timeout = time.Duration(n) * time.Second
		}
	}

	// Build command: handle compose as "docker compose"
	finalArgs := make([]string, 0, len(cmdArgs)+2)
	if command == "compose" {
		finalArgs = append(finalArgs, "compose")
		finalArgs = append(finalArgs, cmdArgs...)
	} else {
		finalArgs = append(finalArgs, command)
		finalArgs = append(finalArgs, cmdArgs...)
	}

	// Add format flags for better output
	finalArgs = t.addFormatDefaults(command, finalArgs, cmdArgs)

	return ExecuteCLICommand(CLICommandInput{
		Ctx:         ctx,
		BinaryPath:  t.binaryPath,
		Args:        finalArgs,
		Timeout:     timeout,
		ToolName:    "docker",
		InstallHint: "Install Docker: https://docs.docker.com/get-docker/",
	})
}

func (t *DockerTool) addFormatDefaults(command string, finalArgs, userArgs []string) []string {
	switch command {
	case "ps":
		if !hasFlag(userArgs, "--format", "") {
			finalArgs = append(finalArgs, "--format", "table {{.ID}}\t{{.Image}}\t{{.Status}}\t{{.Names}}\t{{.Ports}}")
		}
	case "images":
		if !hasFlag(userArgs, "--format", "") {
			finalArgs = append(finalArgs, "--format", "table {{.Repository}}\t{{.Tag}}\t{{.Size}}\t{{.CreatedSince}}")
		}
	case "inspect":
		// JSON by default
		if !hasFlag(userArgs, "--format", "-f") {
			// docker inspect already outputs JSON by default
		}
	}
	return finalArgs
}

// DockerComposeTool is a convenience alias focused on docker compose operations.
type DockerComposeTool struct {
	binaryPath string
	timeout    time.Duration
}

func NewDockerComposeTool(binaryPath string) *DockerComposeTool {
	if binaryPath == "" {
		binaryPath = "docker"
	}
	return &DockerComposeTool{binaryPath: binaryPath, timeout: 120 * time.Second}
}

func (t *DockerComposeTool) Name() string { return "docker_compose" }
func (t *DockerComposeTool) Description() string {
	return "Manage Docker Compose services. Commands: up, down, ps, logs, restart, build, config, exec."
}

func (t *DockerComposeTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type": "string",
				"enum": []string{"up", "down", "ps", "logs", "restart", "build", "config", "exec", "pull", "stop"},
			},
			"args": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
				"description": "Additional compose arguments",
			},
			"file": map[string]any{
				"type":        "string",
				"description": "Path to docker-compose.yml file",
			},
		},
		"required": []string{"action"},
	}
}

func (t *DockerComposeTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	action, _ := args["action"].(string)
	if action == "" {
		return ErrorResult("action is required")
	}

	finalArgs := []string{"compose"}

	if file, ok := args["file"].(string); ok && file != "" {
		finalArgs = append(finalArgs, "-f", file)
	}

	finalArgs = append(finalArgs, action)

	if raw, ok := args["args"]; ok {
		parsed, err := parseStringArgs(raw)
		if err == nil {
			finalArgs = append(finalArgs, parsed...)
		}
	}

	// Auto-add -d for 'up' if not specified
	if action == "up" && !hasFlag(finalArgs, "-d", "--detach") {
		finalArgs = append(finalArgs, "-d")
	}

	return ExecuteCLICommand(CLICommandInput{
		Ctx:         ctx,
		BinaryPath:  t.binaryPath,
		Args:        finalArgs,
		Timeout:     t.timeout,
		ToolName:    "docker compose",
		InstallHint: "Install Docker: https://docs.docker.com/get-docker/",
	})
}
