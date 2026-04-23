package gateway

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/grasberg/sofia/cmd/sofia/internal"
	"github.com/grasberg/sofia/pkg/agent"
	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/channels"
	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/cron"
	"github.com/grasberg/sofia/pkg/devices"
	"github.com/grasberg/sofia/pkg/health"
	"github.com/grasberg/sofia/pkg/heartbeat"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/state"
	"github.com/grasberg/sofia/pkg/tools"
	"github.com/grasberg/sofia/pkg/voice"
	"github.com/grasberg/sofia/pkg/web"
)

func gatewayCmd(debug bool) error {
	if debug {
		logger.SetLevel(logger.DEBUG)
		fmt.Println("🔍 Debug mode enabled")
	}

	cfg, err := internal.LoadConfig()
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	// Load models from DB before creating the provider — config.json no
	// longer stores model_list, so cfg.ModelList is empty until we seed/load.
	if dbPath := cfg.MemoryDBPath(); dbPath != "" {
		if earlyDB, dbErr := memory.Open(dbPath); dbErr == nil {
			if initErr := earlyDB.InitModels(cfg); initErr != nil {
				fmt.Printf("⚠  Failed to load models from DB: %v\n", initErr)
			}
			_ = earlyDB // closed when NewAgentLoop opens its own handle
		}
	}

	provider, _, err := providers.CreateProvider(cfg)
	if err != nil {
		return fmt.Errorf("error creating provider: %w", err)
	}

	if provider == nil {
		fmt.Println("⚠  No default model configured. Start the Web UI and configure a model to begin.")
	}

	msgBus := bus.NewMessageBus()
	agentLoop := agent.NewAgentLoop(cfg, msgBus, provider)

	// Print agent startup info
	fmt.Println("\n📦 Agent Status:")
	startupInfo := agentLoop.GetStartupInfo()
	toolsInfo := startupInfo["tools"].(map[string]any)
	skillsInfo := startupInfo["skills"].(map[string]any)
	fmt.Printf("  • Tools: %d loaded\n", toolsInfo["count"])
	fmt.Printf("  • Skills: %d/%d available\n",
		skillsInfo["available"],
		skillsInfo["total"])

	// Log to file as well
	logger.InfoCF("agent", "Agent initialized",
		map[string]any{
			"tools_count":      toolsInfo["count"],
			"skills_total":     skillsInfo["total"],
			"skills_available": skillsInfo["available"],
		})

	// Setup cron tool and service
	execTimeout := time.Duration(cfg.Tools.Cron.ExecTimeoutMinutes) * time.Minute
	cronService := setupCronTool(
		agentLoop,
		msgBus,
		cfg.WorkspacePath(),
		cfg.Agents.Defaults.RestrictToWorkspace,
		execTimeout,
		cfg,
	)

	heartbeatService := heartbeat.NewHeartbeatService(
		cfg.WorkspacePath(),
		cfg.Heartbeat,
	)
	heartbeatService.SetBus(msgBus)
	heartbeatService.SetHandler(func(prompt, channel, chatID string) *tools.ToolResult {
		// Use cli:direct as fallback if no valid channel
		if channel == "" || chatID == "" {
			channel, chatID = "cli", "direct"
		}
		// Use ProcessHeartbeat - no session history, each heartbeat is independent
		var response string
		response, err = agentLoop.ProcessHeartbeat(context.Background(), prompt, channel, chatID)
		if err != nil {
			return tools.ErrorResult(fmt.Sprintf("Heartbeat error: %v", err))
		}
		if response == "HEARTBEAT_OK" {
			return tools.SilentResult("Heartbeat OK")
		}
		// For heartbeat, always return silent - the subagent result will be
		// sent to user via processSystemMessage when the async task completes
		return tools.SilentResult(response)
	})

	channelManager, err := channels.NewManager(cfg, msgBus)
	if err != nil {
		return fmt.Errorf("error creating channel manager: %w", err)
	}

	// Inject channel manager into agent loop for command handling
	agentLoop.SetChannelManager(channelManager)

	var transcriber *voice.GroqTranscriber
	groqAPIKey := cfg.Providers.Groq.APIKey
	if groqAPIKey == "" {
		for _, mc := range cfg.ModelList {
			if strings.HasPrefix(mc.Model, "groq/") && mc.APIKey != "" {
				groqAPIKey = mc.APIKey
				break
			}
		}
	}
	if groqAPIKey != "" {
		transcriber = voice.NewGroqTranscriber(groqAPIKey)
		logger.InfoC("voice", "Groq voice transcription enabled")
	}

	if transcriber != nil {
		if telegramChannel, ok := channelManager.GetChannel("telegram"); ok {
			if tc, ok := telegramChannel.(*channels.TelegramChannel); ok {
				tc.SetTranscriber(transcriber)
				logger.InfoC("voice", "Groq transcription attached to Telegram channel")
			}
		}
		if discordChannel, ok := channelManager.GetChannel("discord"); ok {
			if dc, ok := discordChannel.(*channels.DiscordChannel); ok {
				dc.SetTranscriber(transcriber)
				logger.InfoC("voice", "Groq transcription attached to Discord channel")
			}
		}
	}

	enabledChannels := channelManager.GetEnabledChannels()
	if len(enabledChannels) > 0 {
		fmt.Printf("✓ Channels enabled: %s\n", enabledChannels)
	} else {
		fmt.Println("⚠ Warning: No channels enabled")
	}

	fmt.Printf("✓ Gateway started on %s:%d\n", cfg.Gateway.Host, cfg.Gateway.Port)
	fmt.Println("Press Ctrl+C to stop")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := cronService.Start(); err != nil {
		fmt.Printf("Error starting cron service: %v\n", err)
	}
	fmt.Println("✓ Cron service started")

	if err := heartbeatService.Start(); err != nil {
		fmt.Printf("Error starting heartbeat service: %v\n", err)
	}
	fmt.Println("✓ Heartbeat service started")

	stateManager := state.NewManager(cfg.WorkspacePath())
	deviceService := devices.NewService(devices.Config{
		Enabled:    cfg.Devices.Enabled,
		MonitorUSB: cfg.Devices.MonitorUSB,
	}, stateManager)
	deviceService.SetBus(msgBus)
	if err := deviceService.Start(ctx); err != nil {
		fmt.Printf("Error starting device service: %v\n", err)
	} else if cfg.Devices.Enabled {
		fmt.Println("✓ Device event service started")
	}

	if err := channelManager.StartAll(ctx); err != nil {
		fmt.Printf("Error starting channels: %v\n", err)
	}

	healthServer := health.NewServer(cfg.Gateway.Host, cfg.Gateway.Port)

	// Register concrete health checks.
	memDB := agentLoop.GetMemoryDB()
	if memDB != nil {
		healthServer.RegisterCheck("database", health.DatabaseCheck(memDB))

		// Wire persistent email dedupe into the email channel so polling is
		// idempotent across restarts. Also wire the support-reply workflow
		// when the email channel is configured as Autonomous=true.
		if emailCh, ok := channelManager.GetChannel("email"); ok {
			if ec, ok := emailCh.(*channels.EmailChannel); ok {
				ec.SetIngestedStore(memDB)

				if ec.Config().Autonomous {
					if err := wireSupportReplyWorkflow(ec, agentLoop, memDB); err != nil {
						logger.ErrorCF("workflows", "support-reply wiring failed",
							map[string]any{"error": err.Error()})
					}
				}
			}
		}
	}
	dataDir := filepath.Dir(cfg.MemoryDBPath())
	healthServer.RegisterCheck("disk_space", health.DiskSpaceCheck(dataDir, 0))

	// Register /metrics endpoint.
	metrics := health.NewMetricsProvider()
	metrics.RegisterMessagesProcessed(func() int64 { return msgBus.InboundCount() })
	if tracker := agentLoop.GetToolTracker(); tracker != nil {
		metrics.RegisterTotalToolCalls(func() int64 { return tracker.TotalCalls() })
	}
	if sm := agentLoop.GetDefaultSessionManager(); sm != nil {
		metrics.RegisterActiveSessions(func() int { return len(sm.ListSessions()) })
	}
	if bm := agentLoop.GetBudgetManager(); bm != nil {
		metrics.RegisterBudgetSpend(func() float64 { return bm.GetTotalSpend() })
	}
	healthServer.RegisterMetrics(metrics)

	go func() {
		if err := healthServer.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.ErrorCF("health", "Health server error", map[string]any{"error": err.Error()})
		}
	}()
	fmt.Printf(
		"✓ Health endpoints available at http://%s:%d/health, /ready, and /metrics\n",
		cfg.Gateway.Host,
		cfg.Gateway.Port,
	)

	if cfg.WebUI.Enabled {
		webServer := web.NewServer(cfg, agentLoop, internal.GetVersion())
		webServer.SetCronService(cronService)
		go func() {
			if err := webServer.Start(ctx); err != nil {
				logger.ErrorCF("web", "Web UI error", map[string]any{"error": err.Error()})
			}
		}()
		fmt.Printf("✓ Web UI available at http://%s:%d\n", cfg.WebUI.Host, cfg.WebUI.Port)
	}

	startGitHubAutonomy(ctx, cfg, agentLoop, memDB, cfg.WorkspacePath())

	go agentLoop.Run(ctx)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	<-sigChan

	fmt.Println("\nShutting down...")
	if cp, ok := provider.(providers.StatefulProvider); ok {
		cp.Close()
	}
	cancel()
	healthServer.Stop(context.Background())
	deviceService.Stop()
	heartbeatService.Stop()
	cronService.Stop()
	agentLoop.Stop()
	channelManager.StopAll(ctx)
	fmt.Println("✓ Gateway stopped")

	return nil
}

func setupCronTool(
	agentLoop *agent.AgentLoop,
	msgBus *bus.MessageBus,
	workspace string,
	restrict bool,
	execTimeout time.Duration,
	cfg *config.Config,
) *cron.CronService {
	cronStorePath := filepath.Join(workspace, "cron", "jobs.json")

	// Create cron service
	cronService := cron.NewCronService(cronStorePath, nil)

	// Create and register CronTool
	cronTool := tools.NewCronTool(cronService, agentLoop, msgBus, workspace, restrict, execTimeout, cfg)
	agentLoop.RegisterTool(cronTool)

	// Set the onJob handler
	cronService.SetOnJob(func(ctx context.Context, job *cron.CronJob) (string, error) {
		result := cronTool.ExecuteJob(ctx, job)
		return result, nil
	})

	return cronService
}
