package triggers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"

	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/logger"
)

// TriggerManager manages event-driven triggers that generate InboundMessages.
type TriggerManager struct {
	bus     *bus.MessageBus
	cfg     *config.Config
	watcher *fsnotify.Watcher
	cancel  context.CancelFunc
	mu      sync.Mutex

	// patternTriggers are compiled regex patterns for message matching
	patternTriggers []compiledPattern
}

type compiledPattern struct {
	regex   *regexp.Regexp
	agentID string
	prompt  string
}

func normalizeTriggerPath(path string) string {
	if strings.HasPrefix(path, "/") {
		return path
	}

	return "/" + path
}

func resolveTriggerAgentID(agentID string) string {
	if agentID == "" {
		return "main"
	}

	return agentID
}

func interpolateTriggerPrompt(prompt string, replacements map[string]string) string {
	result := prompt
	for key, value := range replacements {
		result = strings.ReplaceAll(result, key, value)
	}

	return result
}

// NewTriggerManager creates a new TriggerManager.
func NewTriggerManager(cfg *config.Config, msgBus *bus.MessageBus) *TriggerManager {
	tm := &TriggerManager{
		bus: msgBus,
		cfg: cfg,
	}

	// Compile pattern triggers
	for _, pt := range cfg.Triggers.Patterns {
		re, err := regexp.Compile(pt.Regex)
		if err != nil {
			logger.WarnCF("triggers", "Invalid pattern trigger regex",
				map[string]any{"regex": pt.Regex, "error": err.Error()})
			continue
		}
		tm.patternTriggers = append(tm.patternTriggers, compiledPattern{
			regex:   re,
			agentID: pt.AgentID,
			prompt:  pt.Prompt,
		})
	}

	return tm
}

// Start begins watching for file changes and other triggers.
func (tm *TriggerManager) Start(ctx context.Context) error {
	ctx, tm.cancel = context.WithCancel(ctx)

	// Start file watchers
	if len(tm.cfg.Triggers.FileWatch) > 0 {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return fmt.Errorf("failed to create file watcher: %w", err)
		}
		tm.watcher = watcher

		for _, fw := range tm.cfg.Triggers.FileWatch {
			if err := watcher.Add(fw.Path); err != nil {
				logger.WarnCF("triggers", "Failed to watch path",
					map[string]any{"path": fw.Path, "error": err.Error()})
			}
		}

		go tm.runFileWatcher(ctx)
	}

	return nil
}

// Stop shuts down all trigger watchers.
func (tm *TriggerManager) Stop() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.cancel != nil {
		tm.cancel()
	}
	if tm.watcher != nil {
		tm.watcher.Close()
	}
}

// RegisterWebhooks registers webhook HTTP handlers on the given mux.
func (tm *TriggerManager) RegisterWebhooks(mux *http.ServeMux) {
	for _, wh := range tm.cfg.Triggers.Webhooks {
		path := normalizeTriggerPath(wh.Path)
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}

			body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
			if err != nil {
				http.Error(w, "Failed to read body", http.StatusBadRequest)
				return
			}

			// Verify HMAC if secret is configured
			if wh.Secret != "" {
				sig := r.Header.Get("X-Hub-Signature-256")
				if sig == "" {
					sig = r.Header.Get("X-Signature")
				}
				if !verifyHMAC(body, sig, wh.Secret) {
					http.Error(w, "Invalid signature", http.StatusUnauthorized)
					return
				}
			}

			content := string(body)
			if content == "" {
				content = fmt.Sprintf("Webhook triggered: %s", path)
			}

			agentID := resolveTriggerAgentID(wh.AgentID)

			tm.bus.PublishInbound(bus.InboundMessage{
				Channel:    "webhook",
				SenderID:   "trigger",
				ChatID:     fmt.Sprintf("webhook:%s", path),
				Content:    content,
				SessionKey: fmt.Sprintf("agent:%s:webhook", agentID),
				Metadata: map[string]string{
					"trigger_type": "webhook",
					"webhook_path": path,
				},
			})

			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, `{"status":"ok"}`)

			logger.InfoCF("triggers", "Webhook triggered",
				map[string]any{"path": path, "body_len": len(body)})
		})
	}
}

// CheckPatternTriggers checks if a message matches any pattern triggers.
// Returns true if a trigger was fired.
func (tm *TriggerManager) CheckPatternTriggers(msg bus.InboundMessage) bool {
	if len(tm.patternTriggers) == 0 {
		return false
	}

	fired := false
	for _, pt := range tm.patternTriggers {
		matches := pt.regex.FindStringSubmatch(msg.Content)
		if matches == nil {
			continue
		}

		prompt := pt.prompt
		if prompt == "" {
			prompt = fmt.Sprintf("Pattern trigger matched: %s\nMessage: %s", pt.regex.String(), msg.Content)
		} else {
			// Replace {{.Match}} with the full match
			prompt = strings.ReplaceAll(prompt, "{{.Match}}", matches[0])
		}

		agentID := resolveTriggerAgentID(pt.agentID)

		tm.bus.PublishInbound(bus.InboundMessage{
			Channel:    "trigger",
			SenderID:   "pattern-trigger",
			ChatID:     msg.ChatID,
			Content:    prompt,
			SessionKey: fmt.Sprintf("agent:%s:trigger", agentID),
			Metadata: map[string]string{
				"trigger_type": "pattern",
				"pattern":      pt.regex.String(),
			},
		})

		fired = true
		logger.InfoCF("triggers", "Pattern trigger fired",
			map[string]any{"pattern": pt.regex.String(), "content_preview": msg.Content[:min(len(msg.Content), 80)]})
	}

	return fired
}

func (tm *TriggerManager) runFileWatcher(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-tm.watcher.Events:
			if !ok {
				return
			}
			tm.handleFileEvent(event)
		case err, ok := <-tm.watcher.Errors:
			if !ok {
				return
			}
			logger.WarnCF("triggers", "File watcher error",
				map[string]any{"error": err.Error()})
		}
	}
}

func (tm *TriggerManager) handleFileEvent(event fsnotify.Event) {
	// Only trigger on create and write events
	if !event.Has(fsnotify.Create) && !event.Has(fsnotify.Write) {
		return
	}

	for _, fw := range tm.cfg.Triggers.FileWatch {
		// Check if event matches this trigger's path
		absPath, _ := filepath.Abs(event.Name)
		absWatchPath, _ := filepath.Abs(fw.Path)

		if !strings.HasPrefix(absPath, absWatchPath) {
			continue
		}

		// Check glob pattern filter
		if fw.Pattern != "" {
			matched, err := filepath.Match(fw.Pattern, filepath.Base(event.Name))
			if err != nil || !matched {
				continue
			}
		}

		eventType := "modified"
		if event.Has(fsnotify.Create) {
			eventType = "created"
		}

		prompt := fw.Prompt
		if prompt == "" {
			prompt = fmt.Sprintf("File %s: %s", eventType, event.Name)
		} else {
			prompt = interpolateTriggerPrompt(prompt, map[string]string{
				"{{.File}}":  event.Name,
				"{{.Event}}": eventType,
			})
		}

		agentID := resolveTriggerAgentID(fw.AgentID)

		tm.bus.PublishInbound(bus.InboundMessage{
			Channel:    "trigger",
			SenderID:   "file-trigger",
			ChatID:     fmt.Sprintf("filetrigger:%s", fw.Path),
			Content:    prompt,
			SessionKey: fmt.Sprintf("agent:%s:trigger", agentID),
			Metadata: map[string]string{
				"trigger_type": "file_watch",
				"file":         event.Name,
				"event":        eventType,
			},
		})

		logger.InfoCF("triggers", "File trigger fired",
			map[string]any{"file": event.Name, "event": eventType})
	}
}

func verifyHMAC(body []byte, signature, secret string) bool {
	if signature == "" {
		return false
	}

	// Strip "sha256=" prefix if present
	signature = strings.TrimPrefix(signature, "sha256=")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expected))
}
