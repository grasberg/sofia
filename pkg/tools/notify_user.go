package tools

import (
	"context"

	"github.com/grasberg/sofia/pkg/notifications"
)

// NotifyUserTool allows the agent to send OS-level desktop push notifications to the user.
type NotifyUserTool struct {
	push *notifications.PushService
}

// NewNotifyUserTool creates a new NotifyUserTool.
func NewNotifyUserTool(push *notifications.PushService) *NotifyUserTool {
	return &NotifyUserTool{
		push: push,
	}
}

func (t *NotifyUserTool) Name() string { return "notify_user" }
func (t *NotifyUserTool) Description() string {
	return "Send an OS-level desktop push notification to the user's screen. Use this ONLY for important proactive alerts (e.g. you found something critical while doing background research, or a long-running hourly scheduled task finished). Do NOT use this for normal chat responses."
}

func (t *NotifyUserTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title": map[string]any{
				"type":        "string",
				"description": "Short title for the notification",
			},
			"message": map[string]any{
				"type":        "string",
				"description": "The message body (keep it relatively short)",
			},
			"alert": map[string]any{
				"type":        "boolean",
				"description": "If true, sends an alert that requires user dismissal (depends on OS). Use sparingly for critical items.",
			},
		},
		"required": []string{"title", "message"},
	}
}

func (t *NotifyUserTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	if t.push == nil {
		return ErrorResult("Push notification service is not available.")
	}

	title, _ := args["title"].(string)
	if title == "" {
		return ErrorResult("title is required")
	}

	message, _ := args["message"].(string)
	if message == "" {
		return ErrorResult("message is required")
	}

	alert, _ := args["alert"].(bool)

	var err error
	if alert {
		err = t.push.Alert(title, message)
	} else {
		err = t.push.Send(title, message)
	}

	if err != nil {
		return ErrorResult("Failed to send notification: " + err.Error())
	}

	return SilentResult("Push notification successfully sent to the user's desktop.")
}
