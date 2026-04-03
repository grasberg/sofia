package health

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// MetricsProvider supplies values for the /metrics endpoint.
// Each field is a function that is called on every request so the metrics
// are always fresh.
type MetricsProvider struct {
	mu sync.RWMutex

	// Uptime is derived from startTime — set automatically.
	startTime time.Time

	// Optional metric sources — set via RegisterXxx helpers.
	messagesProcessedFn func() int64
	totalToolCallsFn    func() int64
	activeSessionsFn    func() int
	budgetSpendFn       func() float64
}

// NewMetricsProvider creates a MetricsProvider with the start time set to now.
func NewMetricsProvider() *MetricsProvider {
	return &MetricsProvider{startTime: time.Now()}
}

// RegisterMessagesProcessed sets the function that returns the total number
// of inbound messages processed.
func (mp *MetricsProvider) RegisterMessagesProcessed(fn func() int64) {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	mp.messagesProcessedFn = fn
}

// RegisterTotalToolCalls sets the function that returns the total tool call count.
func (mp *MetricsProvider) RegisterTotalToolCalls(fn func() int64) {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	mp.totalToolCallsFn = fn
}

// RegisterActiveSessions sets the function that returns the active session count.
func (mp *MetricsProvider) RegisterActiveSessions(fn func() int) {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	mp.activeSessionsFn = fn
}

// RegisterBudgetSpend sets the function that returns total budget spend.
func (mp *MetricsProvider) RegisterBudgetSpend(fn func() float64) {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	mp.budgetSpendFn = fn
}

// metricsResponse is the JSON shape served at /metrics.
type metricsResponse struct {
	UptimeSeconds        float64 `json:"uptime_seconds"`
	TotalMessagesProcessed int64   `json:"total_messages_processed"`
	TotalToolCalls       int64   `json:"total_tool_calls"`
	ActiveSessions       int     `json:"active_sessions"`
	BudgetSpendUSD       float64 `json:"budget_spend_usd"`
}

// Handler returns an http.HandlerFunc that serves the /metrics endpoint.
func (mp *MetricsProvider) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mp.mu.RLock()
		defer mp.mu.RUnlock()

		resp := metricsResponse{
			UptimeSeconds: time.Since(mp.startTime).Seconds(),
		}

		if mp.messagesProcessedFn != nil {
			resp.TotalMessagesProcessed = mp.messagesProcessedFn()
		}
		if mp.totalToolCallsFn != nil {
			resp.TotalToolCalls = mp.totalToolCallsFn()
		}
		if mp.activeSessionsFn != nil {
			resp.ActiveSessions = mp.activeSessionsFn()
		}
		if mp.budgetSpendFn != nil {
			resp.BudgetSpendUSD = mp.budgetSpendFn()
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}
}
