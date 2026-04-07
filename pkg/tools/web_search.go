package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Pre-compiled regexes for DuckDuckGo result extraction
var (
	reDDGLink    = regexp.MustCompile(`<a[^>]*class="[^"]*result__a[^"]*"[^>]*href="([^"]+)"[^>]*>([\s\S]*?)</a>`)
	reDDGSnippet = regexp.MustCompile(`<a class="result__snippet[^"]*".*?>([\s\S]*?)</a>`)
)

type SearchProvider interface {
	Search(ctx context.Context, query string, count int) (string, error)
}

type searchRequest struct {
	method  string
	url     string
	body    []byte
	headers map[string]string
	timeout time.Duration
	proxy   string
}

type searchResponse struct {
	statusCode int
	body       []byte
}

type searchResult struct {
	title   string
	url     string
	summary string
}

func executeSearchRequest(ctx context.Context, request searchRequest) (*searchResponse, error) {
	var bodyReader io.Reader
	if len(request.body) > 0 {
		bodyReader = bytes.NewReader(request.body)
	}

	req, err := http.NewRequestWithContext(ctx, request.method, request.url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for key, value := range request.headers {
		req.Header.Set(key, value)
	}

	client, err := createHTTPClient(request.proxy, request.timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP client: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return &searchResponse{statusCode: resp.StatusCode, body: body}, nil
}

func formatWebSearchResults(query, provider string, results []searchResult, count int) string {
	if len(results) == 0 {
		return fmt.Sprintf("No results for: %s", query)
	}

	header := fmt.Sprintf("Results for: %s", query)
	if provider != "" {
		header += fmt.Sprintf(" (via %s)", provider)
	}

	lines := []string{header}
	for i, item := range results {
		if i >= count {
			break
		}

		lines = append(lines, fmt.Sprintf("%d. %s\n   %s", i+1, item.title, item.url))
		if item.summary != "" {
			lines = append(lines, fmt.Sprintf("   %s", item.summary))
		}
	}

	return strings.Join(lines, "\n")
}

type BraveSearchProvider struct {
	apiKey string
	proxy  string
}

func (p *BraveSearchProvider) Search(ctx context.Context, query string, count int) (string, error) {
	searchURL := fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?q=%s&count=%d",
		url.QueryEscape(query), count)

	response, err := executeSearchRequest(ctx, searchRequest{
		method: http.MethodGet,
		url:    searchURL,
		headers: map[string]string{
			"Accept":               "application/json",
			"X-Subscription-Token": p.apiKey,
		},
		timeout: 10 * time.Second,
		proxy:   p.proxy,
	})
	if err != nil {
		return "", err
	}

	var searchResp struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}

	if err := json.Unmarshal(response.body, &searchResp); err != nil {
		// Log error body for debugging
		fmt.Printf("Brave API Error Body: %s\n", string(response.body))
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	results := make([]searchResult, 0, len(searchResp.Web.Results))
	for _, item := range searchResp.Web.Results {
		results = append(results, searchResult{
			title:   item.Title,
			url:     item.URL,
			summary: item.Description,
		})
	}

	return formatWebSearchResults(query, "", results, count), nil
}

type TavilySearchProvider struct {
	apiKey  string
	baseURL string
	proxy   string
}

func (p *TavilySearchProvider) Search(ctx context.Context, query string, count int) (string, error) {
	searchURL := p.baseURL
	if searchURL == "" {
		searchURL = "https://api.tavily.com/search"
	}

	payload := map[string]any{
		"api_key":             p.apiKey,
		"query":               query,
		"search_depth":        "advanced",
		"include_answer":      false,
		"include_images":      false,
		"include_raw_content": false,
		"max_results":         count,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	response, err := executeSearchRequest(ctx, searchRequest{
		method: http.MethodPost,
		url:    searchURL,
		body:   bodyBytes,
		headers: map[string]string{
			"Content-Type": "application/json",
			"User-Agent":   userAgent,
		},
		timeout: 10 * time.Second,
		proxy:   p.proxy,
	})
	if err != nil {
		return "", err
	}

	if response.statusCode != http.StatusOK {
		return "", fmt.Errorf("tavily api error (status %d): %s", response.statusCode, string(response.body))
	}

	var searchResp struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}

	if err := json.Unmarshal(response.body, &searchResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	results := make([]searchResult, 0, len(searchResp.Results))
	for _, item := range searchResp.Results {
		results = append(results, searchResult{
			title:   item.Title,
			url:     item.URL,
			summary: item.Content,
		})
	}

	return formatWebSearchResults(query, "Tavily", results, count), nil
}

type DuckDuckGoSearchProvider struct {
	proxy string
}

func (p *DuckDuckGoSearchProvider) Search(ctx context.Context, query string, count int) (string, error) {
	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))

	response, err := executeSearchRequest(ctx, searchRequest{
		method: http.MethodGet,
		url:    searchURL,
		headers: map[string]string{
			"User-Agent": userAgent,
		},
		timeout: 10 * time.Second,
		proxy:   p.proxy,
	})
	if err != nil {
		return "", err
	}

	return p.extractResults(string(response.body), count, query)
}

func (p *DuckDuckGoSearchProvider) extractResults(html string, count int, query string) (string, error) {
	// Simple regex based extraction for DDG HTML
	// Strategy: Find all result containers or key anchors directly

	// Try finding the result links directly first, as they are the most critical
	// Pattern: <a class="result__a" href="...">Title</a>
	// The previous regex was a bit strict. Let's make it more flexible for attributes order/content
	matches := reDDGLink.FindAllStringSubmatch(html, count+5)

	if len(matches) == 0 {
		return fmt.Sprintf("No results found or extraction failed. Query: %s", query), nil
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Results for: %s (via DuckDuckGo)", query))

	// Pre-compile snippet regex to run inside the loop
	// We'll search for snippets relative to the link position or just globally if needed
	// But simple global search for snippets might mismatch order.
	// Since we only have the raw HTML string, let's just extract snippets globally and assume order matches (risky but simple for regex)
	// Or better: Let's assume the snippet follows the link in the HTML

	// A better regex approach: iterate through text and find matches in order
	// But for now, let's grab all snippets too
	snippetMatches := reDDGSnippet.FindAllStringSubmatch(html, count+5)

	maxItems := min(len(matches), count)

	for i := 0; i < maxItems; i++ {
		urlStr := matches[i][1]
		title := stripTags(matches[i][2])
		title = strings.TrimSpace(title)

		// URL decoding if needed
		if strings.Contains(urlStr, "uddg=") {
			if u, err := url.QueryUnescape(urlStr); err == nil {
				idx := strings.Index(u, "uddg=")
				if idx != -1 {
					urlStr = u[idx+5:]
				}
			}
		}

		lines = append(lines, fmt.Sprintf("%d. %s\n   %s", i+1, title, urlStr))

		// Attempt to attach snippet if available and index aligns
		if i < len(snippetMatches) {
			snippet := stripTags(snippetMatches[i][1])
			snippet = strings.TrimSpace(snippet)
			if snippet != "" {
				lines = append(lines, fmt.Sprintf("   %s", snippet))
			}
		}
	}

	return strings.Join(lines, "\n"), nil
}

func stripTags(content string) string {
	return reTags.ReplaceAllString(content, "")
}

type PerplexitySearchProvider struct {
	apiKey string
	proxy  string
}

func (p *PerplexitySearchProvider) Search(ctx context.Context, query string, count int) (string, error) {
	searchURL := "https://api.perplexity.ai/chat/completions"

	payload := map[string]any{
		"model": "sonar",
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": "You are a search assistant. Provide concise search results with titles, URLs, and brief descriptions in the following format:\n1. Title\n   URL\n   Description\n\nDo not add extra commentary.",
			},
			{
				"role":    "user",
				"content": fmt.Sprintf("Search for: %s. Provide up to %d relevant results.", query, count),
			},
		},
		"max_tokens": 1000,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	response, err := executeSearchRequest(ctx, searchRequest{
		method: http.MethodPost,
		url:    searchURL,
		body:   payloadBytes,
		headers: map[string]string{
			"Content-Type":  "application/json",
			"Authorization": "Bearer " + p.apiKey,
			"User-Agent":    userAgent,
		},
		timeout: 30 * time.Second,
		proxy:   p.proxy,
	})
	if err != nil {
		return "", err
	}

	if response.statusCode != http.StatusOK {
		return "", fmt.Errorf("Perplexity API error: %s", string(response.body))
	}

	var searchResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(response.body, &searchResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if len(searchResp.Choices) == 0 {
		return fmt.Sprintf("No results for: %s", query), nil
	}

	return fmt.Sprintf("Results for: %s (via Perplexity)\n%s", query, searchResp.Choices[0].Message.Content), nil
}

type WebSearchTool struct {
	provider   SearchProvider
	maxResults int
}

type WebSearchToolOptions struct {
	BraveAPIKey          string
	BraveMaxResults      int
	BraveEnabled         bool
	TavilyAPIKey         string
	TavilyBaseURL        string
	TavilyMaxResults     int
	TavilyEnabled        bool
	DuckDuckGoMaxResults int
	DuckDuckGoEnabled    bool
	PerplexityAPIKey     string
	PerplexityMaxResults int
	PerplexityEnabled    bool
	Proxy                string
}

func NewWebSearchTool(opts WebSearchToolOptions) *WebSearchTool {
	var provider SearchProvider
	maxResults := 5

	// Priority: Perplexity > Brave > Tavily > DuckDuckGo
	if opts.PerplexityEnabled && opts.PerplexityAPIKey != "" {
		provider = &PerplexitySearchProvider{apiKey: opts.PerplexityAPIKey, proxy: opts.Proxy}
		if opts.PerplexityMaxResults > 0 {
			maxResults = opts.PerplexityMaxResults
		}
	} else if opts.BraveEnabled && opts.BraveAPIKey != "" {
		provider = &BraveSearchProvider{apiKey: opts.BraveAPIKey, proxy: opts.Proxy}
		if opts.BraveMaxResults > 0 {
			maxResults = opts.BraveMaxResults
		}
	} else if opts.TavilyEnabled && opts.TavilyAPIKey != "" {
		provider = &TavilySearchProvider{
			apiKey:  opts.TavilyAPIKey,
			baseURL: opts.TavilyBaseURL,
			proxy:   opts.Proxy,
		}
		if opts.TavilyMaxResults > 0 {
			maxResults = opts.TavilyMaxResults
		}
	} else if opts.DuckDuckGoEnabled {
		provider = &DuckDuckGoSearchProvider{proxy: opts.Proxy}
		if opts.DuckDuckGoMaxResults > 0 {
			maxResults = opts.DuckDuckGoMaxResults
		}
	} else {
		return nil
	}

	return &WebSearchTool{
		provider:   provider,
		maxResults: maxResults,
	}
}

func (t *WebSearchTool) Name() string {
	return "web_search"
}

func (t *WebSearchTool) Description() string {
	return "Search the web for current information. Returns titles, URLs, and snippets from search results."
}

func (t *WebSearchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search query",
			},
			"count": map[string]any{
				"type":        "integer",
				"description": "Number of results (1-10)",
				"minimum":     1.0,
				"maximum":     10.0,
			},
		},
		"required": []string{"query"},
	}
}

func (t *WebSearchTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	query, ok := args["query"].(string)
	if !ok {
		return ErrorResult("query is required")
	}

	count := t.maxResults
	if c, ok := args["count"].(float64); ok {
		if int(c) > 0 && int(c) <= 10 {
			count = int(c)
		}
	}

	result, err := t.provider.Search(ctx, query, count)
	if err != nil {
		return ErrorResult(fmt.Sprintf("search failed: %v", err))
	}

	return &ToolResult{
		ForLLM:  result,
		ForUser: result,
	}
}
