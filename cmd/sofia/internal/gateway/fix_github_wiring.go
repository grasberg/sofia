package gateway

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/grasberg/sofia/pkg/agent"
	"github.com/grasberg/sofia/pkg/autonomy"
	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/workflows"
)

// startGitHubAutonomy builds the fix-github-issue workflow + poller and
// runs the poller loop in a goroutine until ctx is cancelled. A no-op when
// cfg.GitHubAutonomy.Enabled is false or Repos is empty — callers always
// pass through so the "off by default" contract holds.
func startGitHubAutonomy(
	ctx context.Context,
	cfg *config.Config,
	agentLoop *agent.AgentLoop,
	memDB *memory.MemoryDB,
	workspacePath string,
) {
	raw := cfg.GitHubAutonomy
	if !raw.Enabled {
		return
	}
	ghCfg := config.DefaultGitHubAutonomy(raw)
	if len(ghCfg.Repos) == 0 {
		logger.WarnC("workflows", "github_autonomy enabled but no repos configured — poller not started")
		return
	}
	if memDB == nil {
		logger.ErrorC("workflows", "github_autonomy requires a memory DB; disabled")
		return
	}

	workspaceRoot := ghCfg.WorkspaceRoot
	if workspaceRoot == "" {
		workspaceRoot = filepath.Join(workspacePath, "github-autonomy")
	}

	deps := workflows.FixGitHubIssueDeps{
		Classifier:     workflows.NewHeuristicIssueClassifier(),
		Cloner:         workflows.NewShellCloner(),
		Tester:         workflows.NewShellTester(),
		Fixer:          workflows.NewNoopFixer(),
		Pusher:         workflows.NewShellPusher(),
		PRCreator:      workflows.NewShellPRCreator(),
		IssueCommenter: workflows.NewShellIssueCommenter(),
		BranchPrefix:   ghCfg.BranchPrefix,
		WorkspaceRoot:  workspaceRoot,
		UseFork:        ghCfg.UseFork,
		Locale:         locaeFromEmailCfg(cfg),
	}

	registry := workflows.NewRegistry()
	if err := workflows.RegisterFixGitHubIssue(registry, deps); err != nil {
		logger.ErrorCF("workflows", "fix-github-issue registration failed",
			map[string]any{"error": err.Error()})
		return
	}

	goalSink := workflows.NewGoalSinkAdapter(autonomy.NewGoalManager(memDB), memDB)
	gate := workflows.NewApprovalGateAdapter(agentLoop.GetApprovalGate())
	runner := workflows.NewRunner(registry, goalSink, gate)

	lister := workflows.NewShellIssueLister()
	store := workflows.NewSemanticGraphProcessedStore(memDB)

	poller, err := workflows.NewGitHubPoller(workflows.GitHubPollerConfig{
		Repos:         ghCfg.Repos,
		Label:         ghCfg.Label,
		MaxConcurrent: ghCfg.MaxConcurrent,
		BranchPrefix:  ghCfg.BranchPrefix,
		CloneRoot:     workspaceRoot,
		UseFork:       ghCfg.UseFork,
		Locale:        deps.Locale,
	}, lister, store, runner)
	if err != nil {
		logger.ErrorCF("workflows", "github poller construction failed",
			map[string]any{"error": err.Error()})
		return
	}

	interval := time.Duration(ghCfg.PollMinutes) * time.Minute
	go poller.Start(ctx, interval)

	logger.InfoCF("workflows", "fix-github-issue poller started",
		map[string]any{
			"repos":         ghCfg.Repos,
			"label":         ghCfg.Label,
			"poll_minutes":  ghCfg.PollMinutes,
			"max_concurrent": ghCfg.MaxConcurrent,
			"use_fork":      ghCfg.UseFork,
			"workspace":     workspaceRoot,
		})

	_ = fmt.Sprintf // reserved for future logging helpers
}

// locaeFromEmailCfg returns the user's preferred locale — derived from the
// email channel config which already carries it. Defaults to "sv" when the
// email channel isn't configured.
func locaeFromEmailCfg(cfg *config.Config) string {
	loc := cfg.Channels.Email.UserLocale
	if loc == "" {
		return "sv"
	}
	return loc
}
