package tools

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
)

type GoogleCLITool struct {
	binaryPath      string
	timeout         time.Duration
	allowedCommands map[string]struct{}
}

const maxBatchIDs = 500

func NewGoogleCLITool(binaryPath string, timeoutSeconds int, allowedCommands []string) *GoogleCLITool {
	if strings.TrimSpace(binaryPath) == "" {
		binaryPath = "gog"
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 90
	}

	allow := make(map[string]struct{}, len(allowedCommands))
	for _, cmd := range allowedCommands {
		normalized := strings.ToLower(strings.TrimSpace(cmd))
		if normalized != "" {
			allow[normalized] = struct{}{}
		}
	}

	return &GoogleCLITool{
		binaryPath:      binaryPath,
		timeout:         time.Duration(timeoutSeconds) * time.Second,
		allowedCommands: allow,
	}
}

func (t *GoogleCLITool) Name() string {
	return "google_cli"
}

func (t *GoogleCLITool) Description() string {
	return "Run gog CLI commands for Google services. " +
		"Gmail commands: gmail search <query>, gmail get <msgId>, gmail send --to <email> --subject <s> --body <b>, " +
		"gmail thread get <threadId>, gmail thread modify <threadId> --read/--unread/--archive/--star/--unstar, " +
		"gmail labels list, gmail messages list. " +
		"Drive commands: drive list, drive upload <path>, drive download <fileId>. " +
		"Calendar commands: calendar list, calendar events list. " +
		"IMPORTANT: modify/read operations use 'gmail thread modify <id>', NOT 'gmail modify'. " +
		"For batch modify/delete over many message IDs, prefer one call using batch_ids to reduce subprocess overhead."
}

func (t *GoogleCLITool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"args": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
				"description": "gog command args, for example [\"gmail\",\"search\",\"is:unread\",\"--max\",\"10\"]",
			},
			"account": map[string]any{
				"type":        "string",
				"description": "Optional account email or alias (maps to gog --account)",
			},
			"json": map[string]any{
				"type":        "boolean",
				"description": "Enable JSON output (default true)",
			},
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"description": "Optional timeout override in seconds",
				"minimum":     1.0,
			},
			"batch_ids": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
				"description": "Optional message IDs for gmail batch commands. Prefer this when operating on many IDs so one invocation can process them together",
			},
		},
		"required": []string{"args"},
	}
}

func (t *GoogleCLITool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	commandArgs, err := parseStringArgs(args["args"])
	if err != nil {
		return ErrorResult(err.Error())
	}

	topLevel := strings.ToLower(strings.TrimSpace(commandArgs[0]))
	if strings.HasPrefix(topLevel, "-") {
		return ErrorResult("args must start with a gog top-level command, for example gmail or drive")
	}

	if len(t.allowedCommands) > 0 {
		if _, ok := t.allowedCommands[topLevel]; !ok {
			return ErrorResult(fmt.Sprintf("command %q is not in allowed_commands", topLevel))
		}
	}

	if batchIDs, ok, batchErr := parseBatchIDs(args["batch_ids"]); batchErr != nil {
		return ErrorResult(batchErr.Error())
	} else if ok {
		var injectErr error
		commandArgs, injectErr = injectBatchIDs(commandArgs, batchIDs)
		if injectErr != nil {
			return ErrorResult(injectErr.Error())
		}
	}

	jsonEnabled := true
	if raw, ok := args["json"]; ok {
		value, ok := raw.(bool)
		if !ok {
			return ErrorResult("json must be a boolean")
		}
		jsonEnabled = value
	}

	timeout := t.timeout
	if raw, ok := args["timeout_seconds"]; ok {
		seconds, ok := parsePositiveInt(raw)
		if !ok {
			return ErrorResult("timeout_seconds must be a positive integer")
		}
		timeout = time.Duration(seconds) * time.Second
	}

	account := ""
	if raw, ok := args["account"]; ok {
		s, ok := raw.(string)
		if !ok {
			return ErrorResult("account must be a string")
		}
		account = strings.TrimSpace(s)
	}

	finalArgs := make([]string, 0, len(commandArgs)+4)
	hasAccountFlag, hasJSONFlag := scanInjectedFlags(commandArgs)
	if account != "" && !hasAccountFlag {
		finalArgs = append(finalArgs, "--account", account)
	}
	if jsonEnabled && !hasJSONFlag {
		finalArgs = append(finalArgs, "--json")
	}
	finalArgs = append(finalArgs, commandArgs...)

	// Execute the CLI command using shared helper
	return ExecuteCLICommand(CLICommandInput{
		Ctx:         ctx,
		BinaryPath:  t.binaryPath,
		Args:        finalArgs,
		Timeout:     timeout,
		ToolName:    "gog",
		InstallHint: "Install gogcli and ensure it is in PATH",
	})
}

func parseStringArgs(raw any) ([]string, error) {
	if raw == nil {
		return nil, fmt.Errorf("args is required")
	}

	switch values := raw.(type) {
	case []string:
		if len(values) == 0 {
			return nil, fmt.Errorf("args must contain at least one command token")
		}
		out := make([]string, 0, len(values))
		for i, v := range values {
			trimmed := strings.TrimSpace(v)
			if trimmed == "" {
				return nil, fmt.Errorf("args[%d] must not be empty", i)
			}
			out = append(out, trimmed)
		}
		return out, nil
	case []any:
		if len(values) == 0 {
			return nil, fmt.Errorf("args must contain at least one command token")
		}
		out := make([]string, 0, len(values))
		for i, v := range values {
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("args[%d] must be a string", i)
			}
			trimmed := strings.TrimSpace(s)
			if trimmed == "" {
				return nil, fmt.Errorf("args[%d] must not be empty", i)
			}
			out = append(out, trimmed)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("args must be an array of strings")
	}
}

func scanInjectedFlags(args []string) (hasAccount bool, hasJSON bool) {
	for _, arg := range args {
		if arg == "--json" {
			hasJSON = true
		}
		if arg == "--account" || strings.HasPrefix(arg, "--account=") {
			hasAccount = true
		}
		if hasAccount && hasJSON {
			return true, true
		}
	}
	return hasAccount, hasJSON
}

func parsePositiveInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		if n > 0 {
			return n, true
		}
	case int64:
		if n > 0 && n <= math.MaxInt {
			return int(n), true
		}
	case float64:
		if n > 0 && math.Trunc(n) == n && n <= float64(math.MaxInt) {
			return int(n), true
		}
	}
	return 0, false
}

func parseBatchIDs(raw any) (ids []string, provided bool, err error) {
	if raw == nil {
		return nil, false, nil
	}
	rawIDs, err := parseStringArgs(raw)
	if err != nil {
		return nil, true, fmt.Errorf("batch_ids %v", err)
	}
	if len(rawIDs) > maxBatchIDs {
		return nil, true, fmt.Errorf("batch_ids too long: maximum %d IDs", maxBatchIDs)
	}

	deduped := make([]string, 0, len(rawIDs))
	seen := make(map[string]struct{}, len(rawIDs))
	for _, id := range rawIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		deduped = append(deduped, id)
	}

	return deduped, true, nil
}

func injectBatchIDs(commandArgs, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return commandArgs, nil
	}
	if len(commandArgs) < 3 {
		return nil, fmt.Errorf("batch_ids requires a gmail batch command")
	}
	if strings.ToLower(commandArgs[0]) != "gmail" || strings.ToLower(commandArgs[1]) != "batch" {
		return nil, fmt.Errorf("batch_ids supports only gmail batch commands")
	}
	action := strings.ToLower(commandArgs[2])
	if action != "modify" && action != "delete" {
		return nil, fmt.Errorf("batch_ids supports only 'gmail batch modify' or 'gmail batch delete'")
	}

	injectAt := len(commandArgs)
	for i := 3; i < len(commandArgs); i++ {
		if strings.HasPrefix(commandArgs[i], "-") {
			injectAt = i
			break
		}
	}

	merged := make([]string, 0, len(commandArgs)+len(ids))
	merged = append(merged, commandArgs[:injectAt]...)
	merged = append(merged, ids...)
	merged = append(merged, commandArgs[injectAt:]...)
	return merged, nil
}

