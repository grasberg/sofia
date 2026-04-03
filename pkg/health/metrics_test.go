package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricsHandler_Defaults(t *testing.T) {
	mp := NewMetricsProvider()
	handler := mp.Handler()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var resp metricsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Greater(t, resp.UptimeSeconds, 0.0)
	assert.Equal(t, int64(0), resp.TotalMessagesProcessed)
	assert.Equal(t, int64(0), resp.TotalToolCalls)
	assert.Equal(t, 0, resp.ActiveSessions)
	assert.Equal(t, 0.0, resp.BudgetSpendUSD)
}

func TestMetricsHandler_WithProviders(t *testing.T) {
	mp := NewMetricsProvider()
	mp.RegisterMessagesProcessed(func() int64 { return 42 })
	mp.RegisterTotalToolCalls(func() int64 { return 100 })
	mp.RegisterActiveSessions(func() int { return 3 })
	mp.RegisterBudgetSpend(func() float64 { return 1.50 })

	handler := mp.Handler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	var resp metricsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, int64(42), resp.TotalMessagesProcessed)
	assert.Equal(t, int64(100), resp.TotalToolCalls)
	assert.Equal(t, 3, resp.ActiveSessions)
	assert.InDelta(t, 1.50, resp.BudgetSpendUSD, 0.001)
}
