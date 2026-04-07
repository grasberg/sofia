package web

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// handleMemoryNotes returns all memory notes as JSON.
func (s *Server) handleMemoryNotes(w http.ResponseWriter, _ *http.Request) {
	memDB := s.agentLoop.GetMemoryDB()
	if memDB == nil {
		s.sendJSONError(w, "memory database not available", http.StatusServiceUnavailable)
		return
	}
	notes, err := memDB.ListNotes()
	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if notes == nil {
		w.Write([]byte("[]"))
		return
	}
	json.NewEncoder(w).Encode(notes)
}

// handleMemoryGraph returns semantic nodes and edges for visualization.
func (s *Server) handleMemoryGraph(w http.ResponseWriter, r *http.Request) {
	memDB := s.agentLoop.GetMemoryDB()
	if memDB == nil {
		s.sendJSONError(w, "memory database not available", http.StatusServiceUnavailable)
		return
	}

	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		agentID = "sofia"
	}

	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	nodes, err := memDB.FindNodes(agentID, "", "%", limit)
	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	edges, err := memDB.ListEdges(agentID, limit*5)
	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type graphNode struct {
		ID          int64  `json:"id"`
		Label       string `json:"label"`
		Name        string `json:"name"`
		Properties  string `json:"properties"`
		AccessCount int    `json:"access_count"`
		CreatedAt   string `json:"created_at"`
	}
	type graphEdge struct {
		SourceID    int64   `json:"source_id"`
		TargetID    int64   `json:"target_id"`
		Relation    string  `json:"relation"`
		Weight      float64 `json:"weight"`
		SourceName  string  `json:"source_name"`
		SourceLabel string  `json:"source_label"`
		TargetName  string  `json:"target_name"`
		TargetLabel string  `json:"target_label"`
	}

	gNodes := make([]graphNode, len(nodes))
	for i, n := range nodes {
		gNodes[i] = graphNode{
			ID: n.ID, Label: n.Label, Name: n.Name,
			Properties: n.Properties, AccessCount: n.AccessCount,
			CreatedAt: n.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}
	gEdges := make([]graphEdge, 0, len(edges))
	for _, e := range edges {
		gEdges = append(gEdges, graphEdge{
			SourceID: e.SourceID, TargetID: e.TargetID,
			Relation: e.Relation, Weight: e.Weight,
			SourceName: e.SourceName, SourceLabel: e.SourceLabel,
			TargetName: e.TargetName, TargetLabel: e.TargetLabel,
		})
	}

	// Also collect agent IDs that have nodes
	agentIDs := s.agentLoop.ListAgentIDs()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"nodes":     gNodes,
		"edges":     gEdges,
		"agent_ids": agentIDs,
	})
}

// handleMemoryReflections returns recent reflections as JSON.
func (s *Server) handleMemoryReflections(w http.ResponseWriter, r *http.Request) {
	memDB := s.agentLoop.GetMemoryDB()
	if memDB == nil {
		s.sendJSONError(w, "memory database not available", http.StatusServiceUnavailable)
		return
	}

	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		agentID = "sofia"
	}
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	reflections, err := memDB.GetRecentReflections(agentID, limit)
	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	stats, _ := memDB.GetReflectionStats(agentID, 30)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"reflections": reflections,
		"stats":       stats,
	})
}
