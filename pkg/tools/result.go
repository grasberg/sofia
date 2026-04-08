package tools

import (
	"encoding/json"
	"fmt"
)

// ToolErrorCode represents standardized error categories for tool execution.
// These enable smarter error handling in the agent loop (e.g., auto-retry on RateLimited).
type ToolErrorCode string

const (
	ErrTimeout          ToolErrorCode = "TIMEOUT"
	ErrPermissionDenied ToolErrorCode = "PERMISSION_DENIED"
	ErrNotFound         ToolErrorCode = "NOT_FOUND"
	ErrRateLimited      ToolErrorCode = "RATE_LIMITED"
	ErrInvalidInput     ToolErrorCode = "INVALID_INPUT"
	ErrNetworkError     ToolErrorCode = "NETWORK_ERROR"
	ErrInternalError    ToolErrorCode = "INTERNAL_ERROR"
	ErrNotImplemented   ToolErrorCode = "NOT_IMPLEMENTED"
	ErrQuotaExceeded    ToolErrorCode = "QUOTA_EXCEEDED"
)

// ToolResult represents the structured return value from tool execution.
// It provides clear semantics for different types of results and supports
// async operations, user-facing messages, and error handling.
type ToolResult struct {
	// ForLLM is the content sent to the LLM for context.
	// Required for all results.
	ForLLM string `json:"for_llm"`

	// ForUser is the content sent directly to the user.
	// If empty, no user message is sent.
	// Silent=true overrides this field.
	ForUser string `json:"for_user,omitempty"`

	// Silent suppresses sending any message to the user.
	// When true, ForUser is ignored even if set.
	Silent bool `json:"silent"`

	// IsError indicates whether the tool execution failed.
	// When true, the result should be treated as an error.
	IsError bool `json:"is_error"`

	// Async indicates whether the tool is running asynchronously.
	// When true, the tool will complete later and notify via callback.
	Async bool `json:"async"`

	// Images is an optional list of base64 data URLs (e.g. "data:image/png;base64,...")
	// to inject into the next LLM message. Used by image_analyze and computer_use.
	Images []string `json:"images,omitempty"`

	// Retryable indicates whether the error is transient and the tool call can be retried.
	// Only meaningful when IsError is true.
	Retryable bool `json:"retryable,omitempty"`

	// RetryHint provides guidance to the LLM on how to retry or work around the error.
	// Only meaningful when IsError is true.
	RetryHint string `json:"retry_hint,omitempty"`

	// ErrorCode provides a standardized error category for smarter agent loop handling.
	// Examples: TIMEOUT, PERMISSION_DENIED, NOT_FOUND, RATE_LIMITED, etc.
	ErrorCode ToolErrorCode `json:"error_code,omitempty"`

	// StructuredData holds optional structured output from tools.
	// Enables downstream tools to consume parsed data without re-parsing strings.
	StructuredData any `json:"structured_data,omitempty"`

	// ContentType indicates the format of StructuredData.
	// Examples: "text", "json", "table", "csv"
	ContentType string `json:"content_type,omitempty"`

	// ConfirmationRequired indicates the tool needs user confirmation before proceeding.
	// When true, the agent loop should pause and request confirmation from the user.
	ConfirmationRequired bool `json:"confirmation_required,omitempty"`

	// ConfirmationPrompt is the message to show the user when confirmation is required.
	ConfirmationPrompt string `json:"confirmation_prompt,omitempty"`

	// Err is the underlying error (not JSON serialized).
	// Used for internal error handling and logging.
	Err error `json:"-"`
}

// NewToolResult creates a basic ToolResult with content for the LLM.
// Use this when you need a simple result with default behavior.
//
// Example:
//
//	result := NewToolResult("File updated successfully")
func NewToolResult(forLLM string) *ToolResult {
	return &ToolResult{
		ForLLM: forLLM,
	}
}

// SilentResult creates a ToolResult that is silent (no user message).
// The content is only sent to the LLM for context.
//
// Use this for operations that should not spam the user, such as:
// - File reads/writes
// - Status updates
// - Background operations
//
// Example:
//
//	result := SilentResult("Config file saved")
func SilentResult(forLLM string) *ToolResult {
	return &ToolResult{
		ForLLM:  forLLM,
		Silent:  true,
		IsError: false,
		Async:   false,
	}
}

// AsyncResult creates a ToolResult for async operations.
// The task will run in the background and complete later.
//
// Use this for long-running operations like:
// - Subagent spawns
// - Background processing
// - External API calls with callbacks
//
// Example:
//
//	result := AsyncResult("Subagent spawned, will report back")
func AsyncResult(forLLM string) *ToolResult {
	return &ToolResult{
		ForLLM:  forLLM,
		Silent:  false,
		IsError: false,
		Async:   true,
	}
}

// ErrorResult creates a ToolResult representing an error.
// Sets IsError=true and includes the error message.
//
// Example:
//
//	result := ErrorResult("Failed to connect to database: connection refused")
func ErrorResult(message string) *ToolResult {
	return &ToolResult{
		ForLLM:  message,
		Silent:  false,
		IsError: true,
		Async:   false,
	}
}

// UserResult creates a ToolResult with content for both LLM and user.
// Both ForLLM and ForUser are set to the same content.
//
// Use this when the user needs to see the result directly:
// - Command execution output
// - Fetched web content
// - Query results
//
// Example:
//
//	result := UserResult("Total files found: 42")
func UserResult(content string) *ToolResult {
	return &ToolResult{
		ForLLM:  content,
		ForUser: content,
		Silent:  false,
		IsError: false,
		Async:   false,
	}
}

// MarshalJSON implements custom JSON serialization.
// The Err field is excluded from JSON output via the json:"-" tag.
func (tr *ToolResult) MarshalJSON() ([]byte, error) {
	type Alias ToolResult
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(tr),
	})
}

// WithError sets the Err field and returns the result for chaining.
// This preserves the error for logging while keeping it out of JSON.
//
// Example:
//
//	result := ErrorResult("Operation failed").WithError(err)
func (tr *ToolResult) WithError(err error) *ToolResult {
	tr.Err = err
	return tr
}

// WithRetryHint marks the error as retryable with a hint for the LLM.
func (tr *ToolResult) WithRetryHint(hint string) *ToolResult {
	tr.Retryable = true
	tr.RetryHint = hint
	return tr
}

// RetryableError creates an error result that is marked as retryable.
func RetryableError(message, hint string) *ToolResult {
	return &ToolResult{
		ForLLM:    fmt.Sprintf("%s\n[TOOL_STATUS: error, retryable: true, hint: %q]", message, hint),
		ForUser:   message,
		IsError:   true,
		Retryable: true,
		RetryHint: hint,
	}
}

// WithErrorCode sets the standardized error code and returns the result for chaining.
func (tr *ToolResult) WithErrorCode(code ToolErrorCode) *ToolResult {
	tr.ErrorCode = code
	// Auto-set Retryable based on error code
	switch code {
	case ErrTimeout, ErrRateLimited, ErrNetworkError:
		tr.Retryable = true
	case ErrPermissionDenied, ErrNotFound, ErrInvalidInput, ErrNotImplemented, ErrQuotaExceeded:
		tr.Retryable = false
	default:
		// ErrInternalError - unknown retryability
	}
	return tr
}

// WithStructuredData attaches structured data to the result for downstream consumption.
func (tr *ToolResult) WithStructuredData(data any, contentType string) *ToolResult {
	tr.StructuredData = data
	tr.ContentType = contentType
	return tr
}

// StructuredResult creates a result with structured data output.
func StructuredResult(data any, contentType, summary string) *ToolResult {
	return &ToolResult{
		ForLLM:         summary,
		StructuredData: data,
		ContentType:    contentType,
		Silent:         false,
		IsError:        false,
	}
}

// CodedError creates an error result with a standardized error code.
func CodedError(message string, code ToolErrorCode) *ToolResult {
	result := &ToolResult{
		ForLLM:    message,
		ForUser:   message,
		IsError:   true,
		ErrorCode: code,
	}

	// Auto-set Retryable based on error code
	switch code {
	case ErrTimeout, ErrRateLimited, ErrNetworkError:
		result.Retryable = true
		result.RetryHint = fmt.Sprintf("This error (%s) is transient. Retry after a brief delay.", code)
	case ErrPermissionDenied:
		result.Retryable = false
		result.RetryHint = "This error indicates insufficient permissions. Do not retry without fixing permissions."
	case ErrNotFound:
		result.Retryable = false
		result.RetryHint = "The requested resource was not found. Verify the identifier or path and try again."
	case ErrInvalidInput:
		result.Retryable = false
		result.RetryHint = "The input provided was invalid. Check the format and try again."
	case ErrQuotaExceeded:
		result.Retryable = true
		result.RetryHint = "Quota exceeded. Retry after the quota resets or reduce the request size."
	}

	return result
}

// NonRetryableError creates an error result that should not be retried.
func NonRetryableError(message string) *ToolResult {
	return &ToolResult{
		ForLLM:    fmt.Sprintf("%s\n[TOOL_STATUS: error, retryable: false]", message),
		ForUser:   message,
		IsError:   true,
		Retryable: false,
	}
}

// ConfirmationResult creates a result that requires user confirmation before proceeding.
func ConfirmationResult(prompt string) *ToolResult {
	return &ToolResult{
		ForLLM:               fmt.Sprintf("[CONFIRMATION_REQUIRED: %s]", prompt),
		Silent:               true,
		ConfirmationRequired: true,
		ConfirmationPrompt:   prompt,
	}
}
