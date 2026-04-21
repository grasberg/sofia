package agent

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/chzyer/readline"

	"github.com/grasberg/sofia/cmd/sofia/internal"
	"github.com/grasberg/sofia/pkg/agent"
	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/web"
)

// spinnerFrames are the animation glyphs cycled by liveStatus. Braille dots
// render cleanly in most monospaced terminals and degrade to spaces on TTYs
// that can't handle them.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// ansiClearLine rewinds to the start of the current line and erases its tail.
// Used by the live-status indicator to rewrite itself in place; keeping it a
// single constant makes the contract with callers ("print starts on a clean
// line") explicit.
const ansiClearLine = "\r\033[K"

func agentCmd(message, sessionKey, model string, debug bool) error {
	if sessionKey == "" {
		sessionKey = "cli:default"
	}

	if debug {
		logger.SetLevel(logger.DEBUG)
		fmt.Println("🔍 Debug mode enabled")
	} else {
		// Silence INFO-level chatter so the spinner isn't shredded by
		// per-iteration "TOOL: started" / "SOFIA: LLM returned" lines on
		// stderr. Warnings and errors still surface.
		logger.SetLevel(logger.WARN)
	}

	cfg, err := internal.LoadConfig()
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	if model != "" {
		cfg.Agents.Defaults.ModelName = model
	}

	provider, _, err := providers.CreateProvider(cfg)
	if err != nil {
		return fmt.Errorf("error creating provider: %w", err)
	}

	msgBus := bus.NewMessageBus()
	agentLoop := agent.NewAgentLoop(cfg, msgBus, provider)

	// Print agent startup info (only for interactive mode)
	startupInfo := agentLoop.GetStartupInfo()
	logger.InfoCF("agent", "Agent initialized",
		map[string]any{
			"tools_count":      startupInfo["tools"].(map[string]any)["count"],
			"skills_total":     startupInfo["skills"].(map[string]any)["total"],
			"skills_available": startupInfo["skills"].(map[string]any)["available"],
		})

	if cfg.WebUI.Enabled {
		webServer := web.NewServer(cfg, agentLoop, internal.GetVersion())
		go func() {
			if err := webServer.Start(context.Background()); err != nil {
				logger.ErrorCF("web", "Web UI error", map[string]any{"error": err.Error()})
			}
		}()
		fmt.Printf("✓ Web UI available at http://%s:%d\n", cfg.WebUI.Host, cfg.WebUI.Port)
	}

	if message != "" {
		return runStreamingTurn(context.Background(), agentLoop, message, sessionKey)
	}

	fmt.Printf("%s Interactive mode (Ctrl+C to cancel a reply, Ctrl+D to exit)\n\n", internal.Logo)
	interactiveMode(agentLoop, sessionKey)

	return nil
}

func interactiveMode(agentLoop *agent.AgentLoop, sessionKey string) {
	prompt := fmt.Sprintf("%s You: ", internal.Logo)

	rl, err := readline.NewEx(&readline.Config{
		Prompt:          prompt,
		HistoryFile:     filepath.Join(os.TempDir(), ".sofia_history"),
		HistoryLimit:    100,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		fmt.Printf("Error initializing readline: %v\n", err)
		fmt.Println("Falling back to simple input mode...")
		simpleInteractiveMode(agentLoop, sessionKey)
		return
	}
	defer rl.Close()

	for {
		line, err := rl.Readline()
		if err != nil {
			if err == readline.ErrInterrupt || err == io.EOF {
				fmt.Println("\nGoodbye!")
				return
			}
			fmt.Printf("Error reading input: %v\n", err)
			continue
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" {
			fmt.Println("Goodbye!")
			return
		}

		if err := runStreamingTurn(context.Background(), agentLoop, input, sessionKey); err != nil {
			if errors.Is(err, context.Canceled) {
				fmt.Println("\n(canceled)")
				continue
			}
			fmt.Printf("Error: %v\n", err)
		}
	}
}

func simpleInteractiveMode(agentLoop *agent.AgentLoop, sessionKey string) {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("%s You: ", internal.Logo)
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Println("\nGoodbye!")
				return
			}
			fmt.Printf("Error reading input: %v\n", err)
			continue
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" {
			fmt.Println("Goodbye!")
			return
		}

		if err := runStreamingTurn(context.Background(), agentLoop, input, sessionKey); err != nil {
			if errors.Is(err, context.Canceled) {
				fmt.Println("\n(canceled)")
				continue
			}
			fmt.Printf("Error: %v\n", err)
		}
	}
}

// runStreamingTurn drives one user-to-agent exchange: it shows a live-status
// spinner, starts the agent loop with a streaming callback that prints text
// tokens as they arrive, and installs a SIGINT trap that cancels the
// in-flight request (without killing the REPL). The spinner is torn down
// the moment the first text delta arrives, so output never overlaps; if a
// turn produces only tool calls the spinner stays up for the whole turn.
func runStreamingTurn(parent context.Context, al *agent.AgentLoop, content, sessionKey string) error {
	reqCtx, cancelReq := context.WithCancel(parent)
	defer cancelReq()

	// Trap Ctrl+C for the duration of this turn only. Readline gates its
	// own ^C behaviour while reading input; during the LLM wait the
	// terminal is back in cooked mode and the signal is delivered here.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)
	go func() {
		select {
		case <-sigCh:
			cancelReq()
		case <-reqCtx.Done():
		}
	}()

	spinCtx, stopSpin := context.WithCancel(reqCtx)
	spinDone := make(chan struct{})
	go func() {
		defer close(spinDone)
		liveStatus(spinCtx, al.GetActiveStatus, os.Stdout)
	}()

	// streamStarted tracks whether we've already torn down the spinner
	// and written the "Sofia: " header. The callback runs synchronously
	// from the agent loop goroutine, so plain bool access is safe.
	streamStarted := false
	beginTextBlock := func() {
		if streamStarted {
			return
		}
		streamStarted = true
		stopSpin()
		<-spinDone
		fmt.Printf("\n%s ", internal.Logo)
	}

	err := al.ProcessDirectStream(reqCtx, content, sessionKey, func(text string, done bool) {
		if done || text == "" {
			return
		}
		beginTextBlock()
		fmt.Print(text)
	})

	if streamStarted {
		fmt.Println()
	} else {
		stopSpin()
		<-spinDone
	}
	return err
}

// liveStatus animates a one-line status indicator until ctx is canceled.
// Output goes to stdout; logger output lands on stderr by default, so the
// two don't fight for the same cursor position under normal CLI usage.
func liveStatus(ctx context.Context, getStatus func() string, w io.Writer) {
	start := time.Now()
	ticker := time.NewTicker(120 * time.Millisecond)
	defer ticker.Stop()

	for i := 0; ; i++ {
		select {
		case <-ctx.Done():
			fmt.Fprint(w, ansiClearLine)
			return
		case <-ticker.C:
			status := getStatus()
			if status == "" || status == "Idle" {
				status = "Thinking..."
			}
			fmt.Fprintf(w, "%s%s %s (%.1fs)",
				ansiClearLine,
				spinnerFrames[i%len(spinnerFrames)],
				status,
				time.Since(start).Seconds())
		}
	}
}

