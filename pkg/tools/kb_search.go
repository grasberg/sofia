package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/grasberg/sofia/pkg/memory"
)

// KBSearchTool queries the knowledge base — the user's past question/answer
// pairs distilled from sent replies. Subagents invoke this when drafting a
// support reply so the response is grounded in how the user has answered
// similar questions before.
type KBSearchTool struct {
	memDB   *memory.MemoryDB
	agentID string // scopes the search; empty means cross-agent
}

// NewKBSearchTool constructs the tool. agentID scopes results to one agent's
// KB; pass "" to search every agent's entries (useful for shared inboxes).
func NewKBSearchTool(memDB *memory.MemoryDB, agentID string) *KBSearchTool {
	return &KBSearchTool{memDB: memDB, agentID: agentID}
}

func (t *KBSearchTool) Name() string { return "kb_search" }

func (t *KBSearchTool) Description() string {
	return "Search the knowledge base of past question/answer pairs distilled from the user's sent replies. " +
		"Use this BEFORE drafting a support reply so the answer is grounded in prior responses. " +
		"Returns up to top_k entries ranked by token overlap with the query."
}

func (t *KBSearchTool) Parameters() map[string]any {
	var schema map[string]any
	_ = json.Unmarshal([]byte(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "Terms to match against past questions/answers. Typically the inbound email subject + key phrases."
			},
			"top_k": {
				"type": "integer",
				"description": "Maximum number of entries to return (1-10). Default 3."
			}
		},
		"required": ["query"]
	}`), &schema)
	return schema
}

func (t *KBSearchTool) Execute(_ context.Context, args map[string]any) *ToolResult {
	if t.memDB == nil {
		return ErrorResult("knowledge base is unavailable (memory database not wired)")
	}

	query, _ := args["query"].(string)
	query = strings.TrimSpace(query)
	if query == "" {
		return ErrorResult("query is required")
	}

	topK := 3
	if v, ok := args["top_k"].(float64); ok && v >= 1 && v <= 10 {
		topK = int(v)
	}

	hits, err := t.memDB.SearchKBEntries(t.agentID, query, topK)
	if err != nil {
		return ErrorResult(fmt.Sprintf("kb search failed: %v", err))
	}
	if len(hits) == 0 {
		return NewToolResult("No KB entries matched the query. Draft a fresh reply and the result will seed the KB.")
	}

	forLLM := buildKBResultForLLM(hits)
	result := NewToolResult(forLLM)
	result.StructuredData = hits
	result.ContentType = "json"
	return result
}

func buildKBResultForLLM(hits []memory.KBEntry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Found %d KB entries:\n\n", len(hits))
	for i, h := range hits {
		fmt.Fprintf(&b, "%d. Q: %s\n   A: %s\n", i+1, h.Question, h.Answer)
		if len(h.Tags) > 0 {
			fmt.Fprintf(&b, "   Tags: %s\n", strings.Join(h.Tags, ", "))
		}
		if h.ReplyCount > 0 {
			fmt.Fprintf(&b, "   Used in %d prior replies.\n", h.ReplyCount)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
