package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/search"
)

// SearchHistoryTool allows the LLM to search through conversation history.
type SearchHistoryTool struct {
	memDB *memory.MemoryDB
}

// NewSearchHistoryTool creates a new SearchHistoryTool backed by the given MemoryDB.
func NewSearchHistoryTool(memDB *memory.MemoryDB) *SearchHistoryTool {
	return &SearchHistoryTool{memDB: memDB}
}

func (t *SearchHistoryTool) Name() string { return "search_history" }

func (t *SearchHistoryTool) Description() string {
	return "Search through conversation history across all sessions. " +
		"Returns matching messages ranked by relevance."
}

func (t *SearchHistoryTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "The search query to find in conversation history",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of results to return (default 10)",
			},
		},
		"required": []string{"query"},
	}
}

func (t *SearchHistoryTool) Execute(_ context.Context, args map[string]any) *ToolResult {
	query, _ := args["query"].(string)
	if strings.TrimSpace(query) == "" {
		return ErrorResult("query parameter is required")
	}

	limit := 10
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	// Fetch candidate messages from the database
	dbRows, err := t.memDB.SearchMessages(query, limit*5)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to search messages: %v", err))
	}

	if len(dbRows) == 0 {
		return SilentResult("No matching messages found.")
	}

	// Convert to search entries for keyword ranking
	entries := make([]search.MessageEntry, len(dbRows))
	for i, r := range dbRows {
		entries[i] = search.MessageEntry{
			SessionKey: r.SessionKey,
			Content:    r.Content,
			Role:       r.Role,
			Timestamp:  r.CreatedAt,
		}
	}

	results := search.KeywordSearch(query, entries, limit)
	if len(results) == 0 {
		return SilentResult("No matching messages found.")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d matching messages:\n\n", len(results)))
	for i, r := range results {
		preview := r.Content
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		sb.WriteString(fmt.Sprintf(
			"%d. [%s] (session: %s, score: %.2f)\n   %s\n\n",
			i+1, r.Role, r.SessionKey, r.Score, preview,
		))
	}

	return SilentResult(sb.String())
}
