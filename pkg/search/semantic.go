package search

import (
	"math"
	"sort"
	"strings"
)

// SearchResult represents a matched conversation excerpt.
type SearchResult struct {
	SessionKey string  `json:"session_key"`
	Content    string  `json:"content"`
	Role       string  `json:"role"`
	Score      float64 `json:"score"`
	Timestamp  string  `json:"timestamp,omitempty"`
}

// MessageEntry is a minimal message for search indexing.
type MessageEntry struct {
	SessionKey string
	Content    string
	Role       string
	Timestamp  string
}

// CosineSimilarity computes cosine similarity between two vectors.
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// KeywordSearch performs simple keyword-based search across messages.
// This is the fallback when embeddings are not available.
func KeywordSearch(query string, messages []MessageEntry, topK int) []SearchResult {
	query = strings.ToLower(query)
	queryWords := strings.Fields(query)
	if len(queryWords) == 0 {
		return nil
	}

	type scored struct {
		entry MessageEntry
		score float64
	}
	var results []scored

	for _, msg := range messages {
		content := strings.ToLower(msg.Content)
		score := 0.0
		for _, word := range queryWords {
			if strings.Contains(content, word) {
				score += 1.0
			}
		}
		if score > 0 {
			results = append(results, scored{entry: msg, score: score / float64(len(queryWords))})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if topK > 0 && len(results) > topK {
		results = results[:topK]
	}

	out := make([]SearchResult, 0, len(results))
	for _, r := range results {
		out = append(out, SearchResult{
			SessionKey: r.entry.SessionKey,
			Content:    r.entry.Content,
			Role:       r.entry.Role,
			Score:      r.score,
			Timestamp:  r.entry.Timestamp,
		})
	}
	return out
}
