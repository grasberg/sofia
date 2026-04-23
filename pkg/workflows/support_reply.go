package workflows

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/grasberg/sofia/pkg/agent"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/memory"
)

// RegisterSupportReply builds the canonical support-reply workflow from the
// provided dependencies and installs it in the registry under the name
// "support-reply". Returns an error when required deps are missing so
// configuration bugs surface at boot.
func RegisterSupportReply(r *Registry, deps SupportReplyDeps) error {
	if r == nil {
		return fmt.Errorf("support_reply: registry is nil")
	}
	if deps.Triager == nil {
		deps.Triager = NewHeuristicTriager(nil, nil)
	}
	if deps.Drafter == nil {
		deps.Drafter = NewTemplateDrafter()
	}
	if deps.KBSearcher == nil {
		return fmt.Errorf("support_reply: KBSearcher is required")
	}
	if deps.Sender == nil {
		return fmt.Errorf("support_reply: Sender is required")
	}
	if deps.DefaultLocale == "" {
		deps.DefaultLocale = "sv"
	}
	if deps.AutoSendPriorityFloor == "" {
		deps.AutoSendPriorityFloor = PriorityP3
	}

	wf := &Workflow{
		Name: WorkflowSupportReply,
		Steps: []WorkflowStep{
			buildTriageStep(deps),
			buildRetrieveStep(deps),
			buildDraftStep(deps),
			buildRiskCheckStep(deps),
			buildSendStep(deps),
			buildArchiveStep(deps),
		},
	}
	return r.Register(wf)
}

// buildTriageStep wraps deps.Triager into a WorkflowStep. Output keys:
// priority, sentiment, summary — plus "subject" and "content" exposed for
// the ApprovalGate's classifier hints downstream.
func buildTriageStep(deps SupportReplyDeps) WorkflowStep {
	return WorkflowStep{
		Name:    StepTriage,
		Retries: 1,
		Run: func(ctx context.Context, sc *StepCtx) (StepResult, error) {
			subject := sc.StringInput(InputSubject)
			body := sc.StringInput(InputBody)
			tri, err := deps.Triager.Triage(ctx, subject, body)
			if err != nil {
				return StepResult{}, fmt.Errorf("triage: %w", err)
			}
			return StepResult{Output: map[string]any{
				OutputPriority:  tri.Priority,
				OutputSentiment: tri.Sentiment,
				OutputSummary:   tri.Summary,
				// Surfaced as hints for the risk classifier (approval gate).
				"subject": subject,
				"content": body,
			}}, nil
		},
	}
}

// buildRetrieveStep searches the KB for answers similar to the inbound.
// The ranked hits land in StepCtx.Output as a []memory.KBEntry under key
// "kb_hits". An empty result is fine — the drafter handles that.
func buildRetrieveStep(deps SupportReplyDeps) WorkflowStep {
	return WorkflowStep{
		Name:    StepRetrieve,
		Retries: 1,
		Run: func(_ context.Context, sc *StepCtx) (StepResult, error) {
			agentID := sc.StringInput(InputAgentID)
			query := buildKBQuery(sc)
			hits, err := deps.KBSearcher.Search(agentID, query, 3)
			if err != nil {
				return StepResult{}, fmt.Errorf("kb search: %w", err)
			}
			return StepResult{Output: map[string]any{
				OutputKBHits: hits,
			}}, nil
		},
	}
}

// buildDraftStep delegates to deps.Drafter. The drafter receives the already-
// computed triage + KB hits so it can ground and tone the response.
func buildDraftStep(deps SupportReplyDeps) WorkflowStep {
	return WorkflowStep{
		Name:    StepDraft,
		Retries: 1,
		Run: func(ctx context.Context, sc *StepCtx) (StepResult, error) {
			locale := sc.StringInput(InputLocale)
			if locale == "" {
				locale = deps.DefaultLocale
			}
			hits, _ := sc.Output[OutputKBHits].([]memory.KBEntry)

			req := DraftRequest{
				From:    sc.StringInput(InputFrom),
				Subject: sc.StringInput(InputSubject),
				Body:    sc.StringInput(InputBody),
				Locale:  locale,
				KBHits:  hits,
				Triage: TriageResult{
					Priority:  sc.StringOutput(OutputPriority),
					Sentiment: sc.StringOutput(OutputSentiment),
					Summary:   sc.StringOutput(OutputSummary),
				},
			}
			draft, err := deps.Drafter.Draft(ctx, req)
			if err != nil {
				return StepResult{}, fmt.Errorf("draft: %w", err)
			}
			draft = strings.TrimSpace(draft)
			if draft == "" {
				return StepResult{}, fmt.Errorf("draft: empty body")
			}
			return StepResult{Output: map[string]any{OutputDraft: draft}}, nil
		},
	}
}

// buildRiskCheckStep runs the configured RiskClassifier against the drafted
// reply and stores the level for the send step to consume. Missing
// classifier → RiskUnknown, which sendStep treats as "needs approval".
func buildRiskCheckStep(deps SupportReplyDeps) WorkflowStep {
	return WorkflowStep{
		Name: StepRiskCheck,
		Run: func(ctx context.Context, sc *StepCtx) (StepResult, error) {
			level := RiskUnknown
			if deps.Classifier != nil {
				level = deps.Classifier.Classify(ctx, agent.ToolCallDescriptor{
					ToolName:  "email_send",
					Arguments: sc.StringOutput(OutputDraft),
					Hints: map[string]string{
						"subject":   sc.StringInput(InputSubject),
						"content":   sc.StringInput(InputBody),
						"sentiment": sc.StringOutput(OutputSentiment),
					},
				})
			}
			return StepResult{Output: map[string]any{
				OutputRiskLevel: string(level),
			}}, nil
		},
	}
}

// buildSendStep blocks on approval when the (risk, priority) combination
// demands it, then invokes the email sender and upserts the KB. It's the
// only step with real side-effects, so extra care is taken around
// idempotency (the upstream receiver deduplicates by message_id).
func buildSendStep(deps SupportReplyDeps) WorkflowStep {
	return WorkflowStep{
		Name: StepSend,
		Run: func(ctx context.Context, sc *StepCtx) (StepResult, error) {
			draft := sc.StringOutput(OutputDraft)
			if draft == "" {
				return StepResult{}, fmt.Errorf("send: no draft to send")
			}
			to := sc.StringInput(InputFrom)
			if to == "" {
				return StepResult{}, fmt.Errorf("send: inbound From is empty")
			}

			risk := agent.RiskLevel(sc.StringOutput(OutputRiskLevel))
			priority := sc.StringOutput(OutputPriority)
			approvalStatus := "auto"

			if needsApproval(risk, priority, deps.AutoSendPriorityFloor) {
				ok, err := sc.RequestApproval(ctx, StepSend, "email_send",
					approvalArgumentsJSON(to, sc.StringInput(InputSubject), draft),
					map[string]string{
						"risk_level": string(risk),
						"priority":   priority,
						"sentiment":  sc.StringOutput(OutputSentiment),
						"subject":    sc.StringInput(InputSubject),
					})
				if err != nil {
					return StepResult{}, fmt.Errorf("send approval: %w", err)
				}
				if !ok {
					return StepResult{}, fmt.Errorf("send: denied by approver")
				}
				approvalStatus = "approved"
			}

			subject := replySubject(sc.StringInput(InputSubject))
			if err := deps.Sender.Send(ctx, to, subject, draft); err != nil {
				return StepResult{}, fmt.Errorf("send: %w", err)
			}

			if deps.KBUpserter != nil {
				agentID := sc.StringInput(InputAgentID)
				tri := TriageResult{
					Priority:  sc.StringOutput(OutputPriority),
					Sentiment: sc.StringOutput(OutputSentiment),
				}
				hits, _ := sc.Output[OutputKBHits].([]memory.KBEntry)
				tags := kbTagsFrom(tri, hits)
				source := kbKeyFor(sc.StringInput(InputMessageID))
				if err := deps.KBUpserter.Upsert(agentID,
					sc.StringInput(InputBody), draft, source, tags); err != nil {
					logger.WarnCF("workflows", "kb upsert failed (send already succeeded)",
						map[string]any{"error": err.Error()})
				}
			}

			return StepResult{Output: map[string]any{
				OutputApproval: approvalStatus,
			}}, nil
		},
	}
}

// buildArchiveStep is best-effort — failure here shouldn't undo the reply.
func buildArchiveStep(deps SupportReplyDeps) WorkflowStep {
	return WorkflowStep{
		Name: StepArchive,
		Run: func(ctx context.Context, sc *StepCtx) (StepResult, error) {
			if deps.Archiver == nil {
				return StepResult{}, nil
			}
			messageID := sc.StringInput(InputMessageID)
			if messageID == "" {
				return StepResult{}, nil
			}
			if err := deps.Archiver.Archive(ctx, messageID); err != nil {
				// Non-fatal — log and move on.
				logger.WarnCF("workflows", "archive failed",
					map[string]any{"message_id": messageID, "error": err.Error()})
			}
			return StepResult{}, nil
		},
	}
}

// needsApproval encodes the auto-vs-approve policy: P1/P2 always ask; medium
// or high risk always ask; low risk on P3+ auto-sends.
func needsApproval(risk agent.RiskLevel, priority, floor string) bool {
	if priority != "" && priorityValue(priority) < priorityValue(floor) {
		return true
	}
	switch risk {
	case RiskMedium, RiskHigh, RiskUnknown:
		return true
	}
	return false
}

// priorityValue turns "P1"→1, "P2"→2, etc. Non-P values become 99 so unknown
// priorities default to "low priority" (auto-send permitted).
func priorityValue(p string) int {
	p = strings.ToUpper(strings.TrimSpace(p))
	if len(p) < 2 || p[0] != 'P' {
		return 99
	}
	n := 0
	for _, r := range p[1:] {
		if r < '0' || r > '9' {
			return 99
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// buildKBQuery concatenates the subject and the first few words of the body
// so KB search has something meaningful to match against.
func buildKBQuery(sc *StepCtx) string {
	subject := sc.StringInput(InputSubject)
	body := firstSentence(sc.StringInput(InputBody), 200)
	return strings.TrimSpace(subject + " " + body)
}

// approvalArgumentsJSON produces a compact JSON the approver UI can preview.
func approvalArgumentsJSON(to, subject, body string) string {
	preview := body
	if len(preview) > 600 {
		preview = preview[:600] + "…"
	}
	payload := map[string]string{
		"to":      to,
		"subject": subject,
		"body":    preview,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(b)
}

// replySubject prepends "Re: " exactly once, preserving any existing Re:.
func replySubject(original string) string {
	s := strings.TrimSpace(original)
	if s == "" {
		return "Re: Support"
	}
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "re:") || strings.HasPrefix(lower, "sv:") {
		return s
	}
	return "Re: " + s
}
