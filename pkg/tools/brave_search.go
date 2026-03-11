package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const braveSearchAPI = "https://api.search.brave.com/res/v1/web/search"

// BraveSearchTool searches the web using the Brave Search API.
type BraveSearchTool struct {
	apiKey string
	client *http.Client
}

// NewBraveSearchTool creates a new Brave Search tool.
func NewBraveSearchTool(apiKey string) *BraveSearchTool {
	return &BraveSearchTool{
		apiKey: apiKey,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (t *BraveSearchTool) Name() string {
	return "brave_search"
}

func (t *BraveSearchTool) Description() string {
	return "Search the web using Brave Search. Returns titles, URLs, and descriptions of web results."
}

func (t *BraveSearchTool) Parameters() map[string]any {
	var schema map[string]any
	_ = json.Unmarshal([]byte(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "The search query (1-400 characters)"
			},
			"count": {
				"type": "integer",
				"description": "Number of results to return (1-20, default 5)"
			},
			"country": {
				"type": "string",
				"description": "2-character country code for results (e.g. SE, US). Default: SE"
			},
			"search_lang": {
				"type": "string",
				"description": "Language code for results (e.g. sv, en). Default: sv"
			},
			"freshness": {
				"type": "string",
				"description": "Filter by freshness: pd (past day), pw (past week), pm (past month), py (past year)"
			}
		},
		"required": ["query"]
	}`), &schema)
	return schema
}

func (t *BraveSearchTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	query, _ := args["query"].(string)
	if query == "" {
		return ErrorResult("query is required")
	}
	if len(query) > 400 {
		return ErrorResult("query must be 400 characters or less")
	}

	count := 5
	if c, ok := args["count"].(float64); ok && c >= 1 && c <= 20 {
		count = int(c)
	}

	country := "SE"
	if c, ok := args["country"].(string); ok && c != "" {
		country = c
	}

	searchLang := "sv"
	if l, ok := args["search_lang"].(string); ok && l != "" {
		searchLang = l
	}

	params := url.Values{}
	params.Set("q", query)
	params.Set("count", fmt.Sprintf("%d", count))
	params.Set("country", country)
	params.Set("search_lang", searchLang)
	params.Set("safesearch", "moderate")

	if freshness, ok := args["freshness"].(string); ok && freshness != "" {
		params.Set("freshness", freshness)
	}

	reqURL := braveSearchAPI + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to create request: %v", err))
	}

	req.Header.Set("X-Subscription-Token", t.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "identity") // Keep it simple; Go handles gzip via transport

	resp, err := t.client.Do(req)
	if err != nil {
		return RetryableError(
			fmt.Sprintf("Brave Search request failed: %v", err),
			"Check network connectivity or try again",
		)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024)) // 512 KB limit
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to read response: %v", err))
	}

	if resp.StatusCode != http.StatusOK {
		return ErrorResult(fmt.Sprintf("Brave Search API error (HTTP %d): %s",
			resp.StatusCode, truncateStr(string(body), 300)))
	}

	// Parse response
	var result braveSearchResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return ErrorResult(fmt.Sprintf("failed to parse response: %v", err))
	}

	// Format results for LLM
	var sb strings.Builder
	if result.Query.Altered != "" {
		sb.WriteString(fmt.Sprintf("(Search corrected to: %s)\n\n", result.Query.Altered))
	}

	if result.Web == nil || len(result.Web.Results) == 0 {
		return NewToolResult("No results found for: " + query)
	}

	for i, r := range result.Web.Results {
		sb.WriteString(fmt.Sprintf("%d. **%s**\n", i+1, r.Title))
		sb.WriteString(fmt.Sprintf("   URL: %s\n", r.URL))
		if r.Description != "" {
			sb.WriteString(fmt.Sprintf("   %s\n", r.Description))
		}
		if r.Age != "" {
			sb.WriteString(fmt.Sprintf("   Age: %s\n", r.Age))
		}
		sb.WriteString("\n")
	}

	return NewToolResult(sb.String())
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// braveSearchResponse is the top-level Brave Search API response.
type braveSearchResponse struct {
	Query braveQuery       `json:"query"`
	Web   *braveWebResults `json:"web"`
}

type braveQuery struct {
	Original string `json:"original"`
	Altered  string `json:"altered"`
}

type braveWebResults struct {
	Results []braveWebResult `json:"results"`
}

type braveWebResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Age         string `json:"age"`
}
