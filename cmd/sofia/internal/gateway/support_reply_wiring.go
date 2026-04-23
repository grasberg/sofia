package gateway

import (
	"context"
	"fmt"

	"github.com/grasberg/sofia/pkg/agent"
	"github.com/grasberg/sofia/pkg/autonomy"
	"github.com/grasberg/sofia/pkg/channels"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/workflows"
)

// wireSupportReplyWorkflow assembles the runtime graph for the
// "support-reply" workflow and installs an OnInbound handler on the email
// channel so autonomous polls trigger it. No-ops gracefully when any piece
// is missing (e.g. sender not configured).
func wireSupportReplyWorkflow(
	ec *channels.EmailChannel,
	agentLoop *agent.AgentLoop,
	memDB *memory.MemoryDB,
) error {
	if ec == nil {
		return fmt.Errorf("email channel is nil")
	}
	if memDB == nil {
		return fmt.Errorf("memory DB is nil")
	}

	sender := ec.Sender()
	if sender == nil {
		return fmt.Errorf("email channel has no sender configured")
	}

	cfg := ec.Config()

	goalSink := workflows.NewGoalSinkAdapter(autonomy.NewGoalManager(memDB), memDB)
	gate := workflows.NewApprovalGateAdapter(agentLoop.GetApprovalGate())
	kb := workflows.NewKBAdapter(memDB)

	archiver := channels.NewGmailArchiver(channels.GmailArchiverOptions{
		BinaryPath: cfg.GogBinary,
		Account:    cfg.Username,
		MarkRead:   cfg.MarkAsReadOnIngest,
	})

	var classifier agent.RiskClassifier
	if ag := agentLoop.GetApprovalGate(); ag != nil {
		// Reuse the gate's own classifier so workflow risk_check and
		// ApprovalGate.RequiresApproval share one ruleset.
		classifier = ag
	}

	deps := workflows.SupportReplyDeps{
		KBSearcher:            kb,
		KBUpserter:            kb,
		Sender:                sender,
		Archiver:              archiver,
		Classifier:            classifier,
		DefaultLocale:         cfg.UserLocale,
		AutoSendPriorityFloor: workflows.PriorityP3,
	}

	registry := workflows.NewRegistry()
	if err := workflows.RegisterSupportReply(registry, deps); err != nil {
		return fmt.Errorf("register support-reply: %w", err)
	}

	runner := workflows.NewRunner(registry, goalSink, gate)

	ec.SetInboundHandler(func(email channels.IncomingEmail) {
		inputs := map[string]any{
			workflows.InputFrom:      email.From,
			workflows.InputSubject:   email.Subject,
			workflows.InputBody:      email.Body,
			workflows.InputMessageID: email.MessageID,
			workflows.InputLocale:    cfg.UserLocale,
			workflows.InputAgentID:   "support",
		}
		_, err := runner.Run(
			context.Background(),
			workflows.WorkflowSupportReply,
			"support",
			fmt.Sprintf("Reply to %s: %s", email.From, truncateSubject(email.Subject)),
			inputs,
		)
		if err != nil {
			logger.WarnCF("workflows", "support-reply run ended with error",
				map[string]any{
					"from":       email.From,
					"message_id": email.MessageID,
					"error":      err.Error(),
				})
		}
	})

	logger.InfoCF("workflows", "support-reply workflow wired",
		map[string]any{
			"account":    cfg.Username,
			"locale":     cfg.UserLocale,
			"mark_read":  cfg.MarkAsReadOnIngest,
			"autonomous": cfg.Autonomous,
		})
	return nil
}

// truncateSubject clips long subjects so goal names stay readable.
func truncateSubject(s string) string {
	if len(s) <= 60 {
		return s
	}
	return s[:60] + "…"
}
