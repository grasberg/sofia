package budget

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBudgetManager_UnderBudget(t *testing.T) {
	limits := map[string]BudgetLimit{
		"agent-1": {MaxCostUSD: 10.0, Period: PeriodDaily},
	}
	bm := NewBudgetManager(limits)

	bm.RecordSpend("agent-1", 3.0)
	status, ok := bm.CheckBudget("agent-1")

	require.True(t, ok, "should be under budget")
	assert.Equal(t, "agent-1", status.AgentID)
	assert.InDelta(t, 3.0, status.SpentUSD, 0.001)
	assert.InDelta(t, 10.0, status.LimitUSD, 0.001)
	assert.InDelta(t, 7.0, status.Remaining, 0.001)
	assert.InDelta(t, 30.0, status.Percentage, 0.001)
	assert.Equal(t, PeriodDaily, status.Period)
}

func TestBudgetManager_OverBudget(t *testing.T) {
	limits := map[string]BudgetLimit{
		"agent-1": {MaxCostUSD: 5.0, Period: PeriodDaily},
	}
	bm := NewBudgetManager(limits)

	bm.RecordSpend("agent-1", 5.0)
	status, ok := bm.CheckBudget("agent-1")

	require.False(t, ok, "should be over budget when spent == limit")
	assert.InDelta(t, 5.0, status.SpentUSD, 0.001)
	assert.InDelta(t, 0.0, status.Remaining, 0.001)
	assert.InDelta(t, 100.0, status.Percentage, 0.001)

	// Further spend should also be denied.
	bm.RecordSpend("agent-1", 1.0)
	status, ok = bm.CheckBudget("agent-1")

	require.False(t, ok)
	assert.InDelta(t, 6.0, status.SpentUSD, 0.001)
	assert.InDelta(t, 0.0, status.Remaining, 0.001)
}

func TestBudgetManager_WarningAt80Percent(t *testing.T) {
	limits := map[string]BudgetLimit{
		"agent-1": {MaxCostUSD: 10.0, Period: PeriodDaily},
	}
	bm := NewBudgetManager(limits)

	var mu sync.Mutex
	var warnings []BudgetStatus
	bm.SetWarningCallback(func(agentID string, status BudgetStatus) {
		mu.Lock()
		defer mu.Unlock()
		warnings = append(warnings, status)
	})

	// Spend below 80 % -- no warning.
	bm.RecordSpend("agent-1", 7.0)
	mu.Lock()
	assert.Empty(t, warnings, "no warning expected below 80%%")
	mu.Unlock()

	// Cross 80 % -- warning fires.
	bm.RecordSpend("agent-1", 1.5)
	mu.Lock()
	require.Len(t, warnings, 1, "warning expected at 80%%+")
	assert.Equal(t, "agent-1", warnings[0].AgentID)
	assert.InDelta(t, 8.5, warnings[0].SpentUSD, 0.001)
	mu.Unlock()

	// Additional spend above 80 % should NOT fire again.
	bm.RecordSpend("agent-1", 0.5)
	mu.Lock()
	assert.Len(t, warnings, 1, "no duplicate warning expected")
	mu.Unlock()
}

func TestBudgetManager_PeriodReset(t *testing.T) {
	limits := map[string]BudgetLimit{
		"agent-1": {MaxCostUSD: 10.0, Period: PeriodDaily},
	}
	bm := NewBudgetManager(limits)

	// Simulate spending yesterday.
	yesterday := time.Now().AddDate(0, 0, -1)
	bm.nowFunc = func() time.Time { return yesterday }
	bm.RecordSpend("agent-1", 9.0)

	// Move clock to today -- period should auto-reset.
	bm.nowFunc = time.Now
	status, ok := bm.CheckBudget("agent-1")

	require.True(t, ok, "new period should be under budget")
	assert.InDelta(t, 0.0, status.SpentUSD, 0.001)
	assert.InDelta(t, 10.0, status.Remaining, 0.001)
}

func TestBudgetManager_NoBudgetSet(t *testing.T) {
	bm := NewBudgetManager(nil)

	// No limit configured -- always allowed.
	bm.RecordSpend("agent-x", 999.0)
	status, ok := bm.CheckBudget("agent-x")

	require.True(t, ok, "agents without a limit should always be allowed")
	assert.Equal(t, "agent-x", status.AgentID)
	assert.InDelta(t, 0.0, status.LimitUSD, 0.001)
}

func TestBudgetManager_GetStatus(t *testing.T) {
	limits := map[string]BudgetLimit{
		"agent-1": {MaxCostUSD: 20.0, Period: PeriodMonthly},
	}
	bm := NewBudgetManager(limits)
	bm.RecordSpend("agent-1", 4.0)
	bm.RecordSpend("agent-1", 6.0)

	status := bm.GetStatus("agent-1")

	assert.Equal(t, "agent-1", status.AgentID)
	assert.InDelta(t, 10.0, status.SpentUSD, 0.001)
	assert.InDelta(t, 20.0, status.LimitUSD, 0.001)
	assert.InDelta(t, 10.0, status.Remaining, 0.001)
	assert.InDelta(t, 50.0, status.Percentage, 0.001)
	assert.Equal(t, PeriodMonthly, status.Period)
}

func TestBudgetManager_WeeklyPeriodReset(t *testing.T) {
	limits := map[string]BudgetLimit{
		"agent-1": {MaxCostUSD: 50.0, Period: PeriodWeekly},
	}
	bm := NewBudgetManager(limits)

	// Spend in previous week.
	lastWeek := time.Now().AddDate(0, 0, -8)
	bm.nowFunc = func() time.Time { return lastWeek }
	bm.RecordSpend("agent-1", 49.0)

	// Advance to current week.
	bm.nowFunc = time.Now
	status, ok := bm.CheckBudget("agent-1")

	require.True(t, ok)
	assert.InDelta(t, 0.0, status.SpentUSD, 0.001)
}

func TestBudgetManager_ManualReset(t *testing.T) {
	limits := map[string]BudgetLimit{
		"agent-1": {MaxCostUSD: 10.0, Period: PeriodDaily},
	}
	bm := NewBudgetManager(limits)
	bm.RecordSpend("agent-1", 10.0)

	_, ok := bm.CheckBudget("agent-1")
	require.False(t, ok, "should be over budget")

	bm.ResetPeriod("agent-1")

	status, ok := bm.CheckBudget("agent-1")
	require.True(t, ok, "should be under budget after reset")
	assert.InDelta(t, 0.0, status.SpentUSD, 0.001)
}

func TestValidatePeriod(t *testing.T) {
	assert.NoError(t, ValidatePeriod("daily"))
	assert.NoError(t, ValidatePeriod("weekly"))
	assert.NoError(t, ValidatePeriod("monthly"))
	assert.Error(t, ValidatePeriod("yearly"))
	assert.Error(t, ValidatePeriod(""))
}
