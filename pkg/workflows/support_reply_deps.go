package workflows

import (
	"context"

	"github.com/grasberg/sofia/pkg/agent"
	"github.com/grasberg/sofia/pkg/memory"
)

// Step-output keys — consolidated here so every step agrees on the schema
// carried through StepCtx.Output / Inputs.
const (
	InputFrom      = "from"
	InputSubject   = "subject"
	InputBody      = "body"
	InputMessageID = "message_id"
	InputLocale    = "locale"
	InputAgentID   = "agent_id"

	OutputPriority  = "priority"
	OutputSentiment = "sentiment"
	OutputSummary   = "summary"
	OutputKBHits    = "kb_hits"
	OutputDraft     = "draft"
	OutputRiskLevel = "risk_level"
	OutputApproval  = "approval_status"

	// Step-name constants keep test assertions and runtime telemetry in sync.
	StepTriage    = "triage"
	StepRetrieve  = "retrieve"
	StepDraft     = "draft"
	StepRiskCheck = "risk_check"
	StepSend      = "send"
	StepArchive   = "archive"

	// WorkflowSupportReply is the canonical name. Use this when invoking Run.
	WorkflowSupportReply = "support-reply"
)

// Priority labels follow conventional incident taxonomy (P1 = urgent/outage,
// P4 = informational). The workflow uses priority together with risk level
// to decide auto-send vs approval.
const (
	PriorityP1 = "P1"
	PriorityP2 = "P2"
	PriorityP3 = "P3"
	PriorityP4 = "P4"
)

// Sentiment labels drive both the classifier hints and the drafter tone.
const (
	SentimentPositive = "positive"
	SentimentNeutral  = "neutral"
	SentimentNegative = "negative"
	SentimentHostile  = "hostile"
)

// TriageResult is emitted by the triage step and consumed by later steps.
type TriageResult struct {
	Priority  string `json:"priority"`
	Sentiment string `json:"sentiment"`
	Summary   string `json:"summary"`
}

// DraftRequest bundles everything the Drafter needs to compose a reply.
type DraftRequest struct {
	From     string
	Subject  string
	Body     string
	Locale   string
	KBHits   []memory.KBEntry
	Triage   TriageResult
}

// Triager classifies an inbound email into a priority + sentiment. A default
// heuristic implementation lives in support_reply_defaults.go; production may
// swap in an LLM-backed version by providing its own Triager.
type Triager interface {
	Triage(ctx context.Context, subject, body string) (TriageResult, error)
}

// Drafter composes the reply body given triage info and KB context. Returns
// the full plaintext reply. Subject/threading headers are the caller's job.
type Drafter interface {
	Draft(ctx context.Context, req DraftRequest) (string, error)
}

// KBSearcher retrieves similar past replies to ground the draft. Empty
// agentID searches across all agents — useful for shared inboxes.
type KBSearcher interface {
	Search(agentID, query string, topK int) ([]memory.KBEntry, error)
}

// KBUpserter persists a successful reply as a KB entry so future drafts can
// reuse it. Tags are optional topic labels chosen by the workflow.
type KBUpserter interface {
	Upsert(agentID, question, answer, source string, tags []string) error
}

// EmailSender is the narrow subset of channels.EmailSender used by the
// workflow. We redeclare it here (instead of importing channels) to keep the
// workflows package testable without a message bus.
type EmailSender interface {
	Send(ctx context.Context, to, subject, body string) error
}

// EmailArchiver marks the inbound message handled in the remote mailbox
// (e.g. removes UNREAD, adds a "handled" label). A no-op implementation is
// acceptable when the channel doesn't support labeling.
type EmailArchiver interface {
	Archive(ctx context.Context, messageID string) error
}

// SupportReplyDeps aggregates every collaborator the workflow calls. Only
// Triager, Drafter, KBSearcher, and Sender are mandatory; the rest are best-
// effort and default to no-ops when nil.
type SupportReplyDeps struct {
	Triager       Triager
	Drafter       Drafter
	KBSearcher    KBSearcher
	KBUpserter    KBUpserter
	Sender        EmailSender
	Archiver      EmailArchiver
	Classifier    agent.RiskClassifier // optional — gates auto-send
	DefaultLocale string               // "sv" (default) or "en"

	// AutoSendPriorityFloor sets the highest priority (smallest P-number) at
	// which we'll still auto-send when risk is low. P1/P2 issues always
	// require approval regardless of risk. Default: "P3".
	AutoSendPriorityFloor string
}

// RiskLevelMedium is re-exported here so step code doesn't need to import
// pkg/agent just for a string constant comparison.
const (
	RiskLow     = agent.RiskLow
	RiskMedium  = agent.RiskMedium
	RiskHigh    = agent.RiskHigh
	RiskUnknown = agent.RiskUnknown
)
