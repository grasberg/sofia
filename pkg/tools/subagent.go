package tools

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/skills"
)

type SubagentTask struct {
	ID            string
	Task          string
	Label         string
	AgentID       string
	OriginChannel string
	OriginChatID  string
	Status        string
	Result        string
	Created       int64
}

// GoalContextFunc returns formatted active goal context for injection into subagent prompts.
type GoalContextFunc func() string

type SubagentManager struct {
	tasks           map[string]*SubagentTask
	mu              sync.RWMutex
	provider        providers.LLMProvider
	defaultModel    string
	agentTaskRunner func(ctx context.Context, agentID, sessionKey, task, originChannel, originChatID string) (string, error)
	bus             *bus.MessageBus
	workspace       string
	tools           *ToolRegistry
	maxIterations   int
	maxTokens       int
	temperature     float64
	hasMaxTokens    bool
	hasTemperature  bool
	skillsLoader    *skills.SkillsLoader
	semanticMatcher *SemanticMatcher
	goalContextFn   GoalContextFunc
	nextID          int
}

func NewSubagentManager(
	provider providers.LLMProvider,
	defaultModel, workspace string,
	bus *bus.MessageBus,
) *SubagentManager {
	return &SubagentManager{
		tasks:         make(map[string]*SubagentTask),
		provider:      provider,
		defaultModel:  defaultModel,
		bus:           bus,
		workspace:     workspace,
		tools:         NewToolRegistry(),
		maxIterations: 10,
		nextID:        1,
	}
}

// SetLLMOptions sets max tokens and temperature for subagent LLM calls.
func (sm *SubagentManager) SetLLMOptions(maxTokens int, temperature float64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.maxTokens = maxTokens
	sm.hasMaxTokens = true
	sm.temperature = temperature
	sm.hasTemperature = true
}

// SetGoalContext sets a function that provides active goal context for subagent prompts.
func (sm *SubagentManager) SetGoalContext(fn GoalContextFunc) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.goalContextFn = fn
}

// SetSkillsLoader sets the skills loader for injecting skills into dynamic subagents.
func (sm *SubagentManager) SetSkillsLoader(loader *skills.SkillsLoader) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.skillsLoader = loader
}

// SetTools sets the tool registry for subagent execution.
// If not set, subagent will have access to the provided tools.
func (sm *SubagentManager) SetTools(tools *ToolRegistry) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.tools = tools
}

// RegisterTool registers a tool for subagent execution.
func (sm *SubagentManager) RegisterTool(tool Tool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.tools.Register(tool)
}

// SetAgentTaskRunner configures delegated execution for targeted agent spawns.
// When set, Spawn/agent_id will execute using that agent's configured runtime
// (model, prompt/template, tools) instead of the manager's default model.
func (sm *SubagentManager) SetAgentTaskRunner(
	runner func(ctx context.Context, agentID, sessionKey, task, originChannel, originChatID string) (string, error),
) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.agentTaskRunner = runner
}

func (sm *SubagentManager) cleanupOldTasksLocked() {
	if len(sm.tasks) <= 100 {
		return
	}
	cutoff := time.Now().Add(-1 * time.Hour).UnixMilli()
	for id, t := range sm.tasks {
		if (t.Status == "completed" || t.Status == "failed" || t.Status == "canceled") && t.Created < cutoff {
			delete(sm.tasks, id)
		}
	}
}

func (sm *SubagentManager) Spawn(
	ctx context.Context,
	task, label, agentID string,
	skillsFilter []string,
	originChannel, originChatID string,
	callback AsyncCallback,
) (string, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.cleanupOldTasksLocked()

	taskID := fmt.Sprintf("subagent-%d", sm.nextID)
	sm.nextID++

	subagentTask := &SubagentTask{
		ID:            taskID,
		Task:          task,
		Label:         label,
		AgentID:       agentID,
		OriginChannel: originChannel,
		OriginChatID:  originChatID,
		Status:        "running",
		Created:       time.Now().UnixMilli(),
	}
	sm.tasks[taskID] = subagentTask

	// Start task in background with context cancellation support
	go sm.runTask(ctx, subagentTask, skillsFilter, callback)

	if label != "" {
		return fmt.Sprintf("Spawned subagent '%s' for task: %s", label, task), nil
	}
	return fmt.Sprintf("Spawned subagent for task: %s", task), nil
}

func (sm *SubagentManager) runTask(
	ctx context.Context,
	task *SubagentTask,
	skillsFilter []string,
	callback AsyncCallback,
) {
	task.Status = "running"
	task.Created = time.Now().UnixMilli()

	// Build system prompt for subagent
	systemPrompt := `You are a focused subagent executing a specific task delegated by a coordinator.

## Rules
1. **Complete the task independently** — do not ask clarifying questions. Make reasonable assumptions.
2. **Use tools for every action** — read files before editing, verify results after changes.
3. **Read before write** — always read a file before modifying it. Never edit blindly.
4. **Minimal changes** — change only what the task requires. Don't refactor, add features, or "improve" beyond scope.
5. **Report evidence** — when done, provide a clear summary with specific file paths, line numbers, and actual outputs from tool calls. No speculation.
6. **Fail honestly** — if you cannot complete the task, explain what you tried and what blocked you.
7. **Be terse** — lead with results, not reasoning. Skip preamble.`

	sm.mu.RLock()
	sLoader := sm.skillsLoader
	goalFn := sm.goalContextFn
	sm.mu.RUnlock()

	// Inject active goal context so the subagent understands WHY this task exists
	if goalFn != nil {
		goalCtx := goalFn()
		if goalCtx != "" {
			systemPrompt += "\n\n# Active Goals\n\nThis task contributes to the following goals:\n" + goalCtx
		}
	}

	if len(skillsFilter) > 0 && sLoader != nil {
		skillsSummary := sLoader.BuildSkillsSummaryFor(skillsFilter)
		if skillsSummary != "" {
			systemPrompt += fmt.Sprintf(
				"\n\n# Skills\n\nThe following skills extend your capabilities. To use a skill, read its SKILL.md file using the read_file tool.\n\n%s",
				skillsSummary,
			)
		}
	}

	messages := []providers.Message{
		{
			Role:    "system",
			Content: systemPrompt,
		},
		{
			Role:    "user",
			Content: task.Task,
		},
	}

	// Check if context is already canceled before starting
	select {
	case <-ctx.Done():
		sm.mu.Lock()
		task.Status = "canceled"
		task.Result = "Task canceled before execution"
		sm.mu.Unlock()
		return
	default:
	}

	// Run tool loop with access to tools
	sm.mu.RLock()
	tools := sm.tools
	maxIter := sm.maxIterations
	maxTokens := sm.maxTokens
	temperature := sm.temperature
	hasMaxTokens := sm.hasMaxTokens
	hasTemperature := sm.hasTemperature
	agentTaskRunner := sm.agentTaskRunner
	sm.mu.RUnlock()

	if task.AgentID != "" && agentTaskRunner != nil {
		content, err := agentTaskRunner(
			ctx,
			task.AgentID,
			"subagent:"+task.ID,
			task.Task,
			task.OriginChannel,
			task.OriginChatID,
		)

		sm.mu.Lock()
		var result *ToolResult

		if err != nil {
			task.Status = "failed"
			task.Result = fmt.Sprintf("Error: %v", err)
			result = &ToolResult{
				ForLLM:  task.Result,
				ForUser: "",
				Silent:  false,
				IsError: true,
				Async:   false,
				Err:     err,
			}
		} else {
			task.Status = "completed"
			task.Result = content
			result = &ToolResult{
				ForLLM:  fmt.Sprintf("Subagent '%s' completed via agent '%s': %s", task.Label, task.AgentID, content),
				ForUser: content,
				Silent:  false,
				IsError: false,
				Async:   false,
			}
		}
		sm.mu.Unlock()

		if sm.bus != nil {
			announceContent := fmt.Sprintf("Task '%s' completed.\n\nResult:\n%s", task.Label, task.Result)
			sm.bus.PublishInbound(bus.InboundMessage{
				Channel:  "system",
				SenderID: fmt.Sprintf("subagent:%s", task.ID),
				ChatID:   fmt.Sprintf("%s:%s", task.OriginChannel, task.OriginChatID),
				Content:  announceContent,
			})
		}

		if callback != nil && result != nil {
			callback(ctx, result)
		}
		return
	}

	var llmOptions map[string]any
	if hasMaxTokens || hasTemperature {
		llmOptions = map[string]any{}
		if hasMaxTokens {
			llmOptions["max_tokens"] = maxTokens
		}
		if hasTemperature {
			llmOptions["temperature"] = temperature
		}
	}

	loopResult, err := RunToolLoop(ctx, ToolLoopConfig{
		Provider:        sm.provider,
		Model:           sm.defaultModel,
		Tools:           tools,
		MaxIterations:   maxIter,
		LLMOptions:      llmOptions,
		SemanticMatcher: sm.semanticMatcher,
		SemanticTopK:    10, // Example config-driven value, defaulting to 10
	}, messages, task.OriginChannel, task.OriginChatID)

	sm.mu.Lock()
	var result *ToolResult
	defer func() {
		sm.mu.Unlock()
		// Call callback if provided and result is set
		if callback != nil && result != nil {
			callback(ctx, result)
		}
	}()

	if err != nil {
		task.Status = "failed"
		task.Result = fmt.Sprintf("Error: %v", err)
		// Check if it was canceled
		if ctx.Err() != nil {
			task.Status = "canceled"
			task.Result = "Task canceled during execution"
		}
		result = &ToolResult{
			ForLLM:  task.Result,
			ForUser: "",
			Silent:  false,
			IsError: true,
			Async:   false,
			Err:     err,
		}
	} else {
		task.Status = "completed"
		task.Result = loopResult.Content
		result = &ToolResult{
			ForLLM: fmt.Sprintf(
				"Subagent '%s' completed (iterations: %d): %s",
				task.Label,
				loopResult.Iterations,
				loopResult.Content,
			),
			ForUser: loopResult.Content,
			Silent:  false,
			IsError: false,
			Async:   false,
		}
	}

	// Send announce message back to main agent
	if sm.bus != nil {
		announceContent := fmt.Sprintf("Task '%s' completed.\n\nResult:\n%s", task.Label, task.Result)
		sm.bus.PublishInbound(bus.InboundMessage{
			Channel:  "system",
			SenderID: fmt.Sprintf("subagent:%s", task.ID),
			// Format: "original_channel:original_chat_id" for routing back
			ChatID:  fmt.Sprintf("%s:%s", task.OriginChannel, task.OriginChatID),
			Content: announceContent,
		})
	}
}

func (sm *SubagentManager) GetTask(taskID string) (*SubagentTask, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	task, ok := sm.tasks[taskID]
	return task, ok
}

func (sm *SubagentManager) ListTasks() []*SubagentTask {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	tasks := make([]*SubagentTask, 0, len(sm.tasks))
	for _, task := range sm.tasks {
		tasks = append(tasks, task)
	}
	return tasks
}
