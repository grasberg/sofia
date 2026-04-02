package budget

import (
	"fmt"
	"sync"
	"time"
)

// BudgetPeriod defines the time window for budget tracking.
type BudgetPeriod string

const (
	PeriodDaily   BudgetPeriod = "daily"
	PeriodWeekly  BudgetPeriod = "weekly"
	PeriodMonthly BudgetPeriod = "monthly"
)

// BudgetLimit defines a spending cap for an agent over a given period.
type BudgetLimit struct {
	MaxCostUSD float64      `json:"max_cost_usd"`
	Period     BudgetPeriod `json:"period"`
}

// BudgetStatus is a point-in-time snapshot of an agent's budget consumption.
type BudgetStatus struct {
	AgentID     string       `json:"agent_id"`
	SpentUSD    float64      `json:"spent_usd"`
	LimitUSD    float64      `json:"limit_usd"`
	Period      BudgetPeriod `json:"period"`
	PeriodStart time.Time    `json:"period_start"`
	Remaining   float64      `json:"remaining_usd"`
	Percentage  float64      `json:"percentage"`
}

// BudgetManager tracks per-agent spending against configurable budget limits.
type BudgetManager struct {
	mu         sync.Mutex
	limits     map[string]BudgetLimit                    // agentID -> limit
	spending   map[string]*spendEntry                    // agentID -> current period spend
	onWarning  func(agentID string, status BudgetStatus) // callback at 80%
	onHardStop func(agentID string, status BudgetStatus) // callback at 100% — agent should be paused
	nowFunc    func() time.Time                          // injectable clock for testing
}

type spendEntry struct {
	Amount      float64
	PeriodStart time.Time
}

const (
	warningThreshold  = 0.80
	hardStopThreshold = 1.00
)

// NewBudgetManager creates a BudgetManager with the supplied per-agent limits.
// Pass a nil map if no limits are needed (all agents will be allowed).
func NewBudgetManager(limits map[string]BudgetLimit) *BudgetManager {
	if limits == nil {
		limits = make(map[string]BudgetLimit)
	}
	return &BudgetManager{
		limits:   limits,
		spending: make(map[string]*spendEntry),
		nowFunc:  time.Now,
	}
}

// SetWarningCallback registers a function that is invoked when an agent's
// spend crosses the 80% threshold within its current period.
func (bm *BudgetManager) SetWarningCallback(fn func(string, BudgetStatus)) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	bm.onWarning = fn
}

// SetHardStopCallback registers a function that is invoked when an agent's
// spend reaches 100% of its budget. The agent should be paused.
func (bm *BudgetManager) SetHardStopCallback(fn func(string, BudgetStatus)) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	bm.onHardStop = fn
}

// RecordSpend adds costUSD to the agent's current-period spend.
// If the period has expired, spending is reset before the new cost is added.
func (bm *BudgetManager) RecordSpend(agentID string, costUSD float64) {
	var cb func(string, BudgetStatus)
	var status BudgetStatus
	var callbackAgentID string

	bm.mu.Lock()

	limit, hasLimit := bm.limits[agentID]
	entry := bm.getOrCreateEntry(agentID, limit.Period)

	// Reset if the period has rolled over.
	if hasLimit && bm.periodExpired(entry.PeriodStart, limit.Period) {
		entry.Amount = 0
		entry.PeriodStart = bm.periodStart(limit.Period)
	}

	entry.Amount += costUSD

	// Collect callback data for threshold crossings.
	var hardCb func(string, BudgetStatus)
	var hardStatus BudgetStatus
	var hardAgentID string

	if hasLimit && limit.MaxCostUSD > 0 {
		pct := entry.Amount / limit.MaxCostUSD
		prevPct := (entry.Amount - costUSD) / limit.MaxCostUSD

		// Hard stop at 100%
		if pct >= hardStopThreshold && prevPct < hardStopThreshold && bm.onHardStop != nil {
			hardCb = bm.onHardStop
			hardStatus = bm.buildStatus(agentID, entry, limit)
			hardAgentID = agentID
		}

		// Warning at 80%
		if pct >= warningThreshold && prevPct < warningThreshold && bm.onWarning != nil {
			cb = bm.onWarning
			status = bm.buildStatus(agentID, entry, limit)
			callbackAgentID = agentID
		}
	}

	bm.mu.Unlock()

	// Invoke callbacks entirely outside the lock to avoid deadlocks.
	if cb != nil {
		cb(callbackAgentID, status)
	}
	if hardCb != nil {
		hardCb(hardAgentID, hardStatus)
	}
}

// CheckBudget returns the current status and whether the agent is still under
// its budget (true = allowed to proceed). Agents with no configured limit are
// always allowed.
func (bm *BudgetManager) CheckBudget(agentID string) (BudgetStatus, bool) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	limit, hasLimit := bm.limits[agentID]
	if !hasLimit {
		return BudgetStatus{AgentID: agentID}, true
	}

	entry := bm.getOrCreateEntry(agentID, limit.Period)
	if bm.periodExpired(entry.PeriodStart, limit.Period) {
		entry.Amount = 0
		entry.PeriodStart = bm.periodStart(limit.Period)
	}

	status := bm.buildStatus(agentID, entry, limit)
	underBudget := entry.Amount < limit.MaxCostUSD
	return status, underBudget
}

// GetStatus returns the current budget status for the given agent.
func (bm *BudgetManager) GetStatus(agentID string) BudgetStatus {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	limit, hasLimit := bm.limits[agentID]
	if !hasLimit {
		return BudgetStatus{AgentID: agentID}
	}

	entry := bm.getOrCreateEntry(agentID, limit.Period)
	if bm.periodExpired(entry.PeriodStart, limit.Period) {
		entry.Amount = 0
		entry.PeriodStart = bm.periodStart(limit.Period)
	}

	return bm.buildStatus(agentID, entry, limit)
}

// ResetPeriod manually resets the spending for an agent.
func (bm *BudgetManager) ResetPeriod(agentID string) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	limit := bm.limits[agentID]
	if entry, ok := bm.spending[agentID]; ok {
		entry.Amount = 0
		entry.PeriodStart = bm.periodStart(limit.Period)
	}
}

// ---------- internal helpers ----------

func (bm *BudgetManager) now() time.Time {
	return bm.nowFunc()
}

func (bm *BudgetManager) getOrCreateEntry(agentID string, period BudgetPeriod) *spendEntry {
	if e, ok := bm.spending[agentID]; ok {
		return e
	}
	e := &spendEntry{PeriodStart: bm.periodStart(period)}
	bm.spending[agentID] = e
	return e
}

// periodStart returns the beginning of the current period relative to now.
func (bm *BudgetManager) periodStart(period BudgetPeriod) time.Time {
	n := bm.now()
	switch period {
	case PeriodDaily:
		return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, n.Location())
	case PeriodWeekly:
		weekday := int(n.Weekday())
		if weekday == 0 { // Sunday
			weekday = 7
		}
		daysSinceMonday := weekday - 1
		monday := n.AddDate(0, 0, -daysSinceMonday)
		return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, n.Location())
	case PeriodMonthly:
		return time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, n.Location())
	default:
		return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, n.Location())
	}
}

// periodExpired reports whether entryStart belongs to a period that has ended.
func (bm *BudgetManager) periodExpired(entryStart time.Time, period BudgetPeriod) bool {
	current := bm.periodStart(period)
	return entryStart.Before(current)
}

func (bm *BudgetManager) buildStatus(agentID string, entry *spendEntry, limit BudgetLimit) BudgetStatus {
	remaining := limit.MaxCostUSD - entry.Amount
	if remaining < 0 {
		remaining = 0
	}
	pct := 0.0
	if limit.MaxCostUSD > 0 {
		pct = (entry.Amount / limit.MaxCostUSD) * 100
	}
	return BudgetStatus{
		AgentID:     agentID,
		SpentUSD:    entry.Amount,
		LimitUSD:    limit.MaxCostUSD,
		Period:      limit.Period,
		PeriodStart: entry.PeriodStart,
		Remaining:   remaining,
		Percentage:  pct,
	}
}

// ValidatePeriod returns an error if period is not a recognized BudgetPeriod.
func ValidatePeriod(period string) error {
	switch BudgetPeriod(period) {
	case PeriodDaily, PeriodWeekly, PeriodMonthly:
		return nil
	default:
		return fmt.Errorf("invalid budget period %q: must be daily, weekly, or monthly", period)
	}
}
