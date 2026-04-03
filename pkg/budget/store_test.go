package budget

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSQLiteStore_SaveAndLoad(t *testing.T) {
	db := openTestDB(t)
	store, err := NewSQLiteStore(db)
	require.NoError(t, err)

	now := time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)
	entries := map[string]*spendEntry{
		"agent-1": {Amount: 5.25, PeriodStart: now},
		"agent-2": {Amount: 12.0, PeriodStart: now.Add(-24 * time.Hour)},
	}

	require.NoError(t, store.Save(entries))

	loaded, err := store.Load()
	require.NoError(t, err)
	require.Len(t, loaded, 2)

	assert.InDelta(t, 5.25, loaded["agent-1"].Amount, 0.001)
	assert.True(t, loaded["agent-1"].PeriodStart.Equal(now))
	assert.InDelta(t, 12.0, loaded["agent-2"].Amount, 0.001)
}

func TestSQLiteStore_UpsertOverwrites(t *testing.T) {
	db := openTestDB(t)
	store, err := NewSQLiteStore(db)
	require.NoError(t, err)

	now := time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)

	// Initial save.
	require.NoError(t, store.Save(map[string]*spendEntry{
		"agent-1": {Amount: 1.0, PeriodStart: now},
	}))

	// Overwrite with new amount.
	require.NoError(t, store.Save(map[string]*spendEntry{
		"agent-1": {Amount: 7.5, PeriodStart: now},
	}))

	loaded, err := store.Load()
	require.NoError(t, err)
	assert.InDelta(t, 7.5, loaded["agent-1"].Amount, 0.001)
}

func TestBudgetManager_PersistenceRoundTrip(t *testing.T) {
	db := openTestDB(t)
	store, err := NewSQLiteStore(db)
	require.NoError(t, err)

	limits := map[string]BudgetLimit{
		"agent-1": {MaxCostUSD: 100.0, Period: PeriodDaily},
	}

	// Create manager, record some spend.
	bm := NewBudgetManager(limits, WithStore(store))
	bm.RecordSpend("agent-1", 42.0)

	status := bm.GetStatus("agent-1")
	assert.InDelta(t, 42.0, status.SpentUSD, 0.001)

	// Simulate restart: new manager with same store.
	bm2 := NewBudgetManager(limits, WithStore(store))
	status2 := bm2.GetStatus("agent-1")
	assert.InDelta(t, 42.0, status2.SpentUSD, 0.001)
}

func TestBudgetManager_GetTotalSpend(t *testing.T) {
	limits := map[string]BudgetLimit{
		"agent-1": {MaxCostUSD: 100.0, Period: PeriodDaily},
		"agent-2": {MaxCostUSD: 50.0, Period: PeriodDaily},
	}
	bm := NewBudgetManager(limits)
	bm.RecordSpend("agent-1", 10.0)
	bm.RecordSpend("agent-2", 5.0)

	total := bm.GetTotalSpend()
	assert.InDelta(t, 15.0, total, 0.001)
}

func TestSQLiteStore_LoadEmpty(t *testing.T) {
	db := openTestDB(t)
	store, err := NewSQLiteStore(db)
	require.NoError(t, err)

	loaded, err := store.Load()
	require.NoError(t, err)
	assert.Empty(t, loaded)
}
