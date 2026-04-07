package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/grasberg/sofia/pkg/eval"
)

// SetEvalStore assigns the eval store used by the /api/eval/* endpoints.
func (s *Server) SetEvalStore(store *eval.EvalStore) {
	s.evalStore = store
}

// handleEvalRuns returns recent eval runs as JSON.
// Query params: suite (filter by suite name), limit (default 20).
func (s *Server) handleEvalRuns(w http.ResponseWriter, r *http.Request) {
	if s.evalStore == nil {
		s.sendJSONError(w, "Eval store not available", http.StatusServiceUnavailable)
		return
	}

	suite := r.URL.Query().Get("suite")
	limit := 20

	if v := r.URL.Query().Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	if suite != "" {
		runs, err := s.evalStore.GetRunHistory(suite, limit)
		if err != nil {
			s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if runs == nil {
			runs = []eval.EvalRunSummary{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(runs)

		return
	}

	// No suite filter: return recent runs across all suites.
	runs, err := s.evalStore.GetRecentRuns(limit)
	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if runs == nil {
		runs = []eval.EvalRunSummary{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(runs)
}

// handleEvalRunDetail returns a single eval run with per-test results.
// Path: GET /api/eval/runs/<id>
func (s *Server) handleEvalRunDetail(w http.ResponseWriter, r *http.Request) {
	if s.evalStore == nil {
		s.sendJSONError(w, "Eval store not available", http.StatusServiceUnavailable)
		return
	}

	idStr := strings.TrimPrefix(r.URL.Path, "/api/eval/runs/")
	if idStr == "" {
		s.sendJSONError(w, "Run ID is required", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.sendJSONError(w, "Invalid run ID", http.StatusBadRequest)
		return
	}

	run, err := s.evalStore.GetRunByID(id)
	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if run == nil {
		s.sendJSONError(w, "Run not found", http.StatusNotFound)
		return
	}

	results, err := s.evalStore.GetRunResults(id)
	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if results == nil {
		results = []eval.EvalResultRow{}
	}

	detail := eval.EvalRunDetail{
		EvalRunSummary: *run,
		Results:        results,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detail)
}

// handleEvalTrend returns trend data for a suite: the last N run scores and a
// trend label (improving/declining/stable/insufficient_data).
// Query params: suite (required), limit (default 10).
func (s *Server) handleEvalTrend(w http.ResponseWriter, r *http.Request) {
	if s.evalStore == nil {
		s.sendJSONError(w, "Eval store not available", http.StatusServiceUnavailable)
		return
	}

	suite := r.URL.Query().Get("suite")
	if suite == "" {
		s.sendJSONError(w, "suite parameter is required", http.StatusBadRequest)
		return
	}

	limit := 10

	if v := r.URL.Query().Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	runs, err := s.evalStore.GetRunHistory(suite, limit)
	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	trend, err := s.evalStore.GetTrend(suite)
	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Build score array (oldest to newest for charting).
	type trendPoint struct {
		RunID    int64   `json:"run_id"`
		AvgScore float64 `json:"avg_score"`
		PassRate float64 `json:"pass_rate"`
		RunAt    string  `json:"run_at"`
	}

	points := make([]trendPoint, 0, len(runs))
	for i := len(runs) - 1; i >= 0; i-- {
		run := runs[i]
		points = append(points, trendPoint{
			RunID:    run.ID,
			AvgScore: run.AvgScore,
			PassRate: run.PassRate,
			RunAt:    run.RunAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"suite":  suite,
		"trend":  trend,
		"points": points,
	})
}
