package tools

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/grasberg/sofia/pkg/providers"
)

// mockEmbeddingProvider is a simple mock for testing the semantic matcher.
type mockEmbeddingProvider struct {
	embeddings map[string][]float32
	err        error
	callCount  int
}

func (m *mockEmbeddingProvider) Chat(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, model string, options map[string]any) (*providers.LLMResponse, error) {
	return nil, nil // not used in these tests
}

func (m *mockEmbeddingProvider) GetDefaultModel() string {
	return "mock"
}

func (m *mockEmbeddingProvider) Embeddings(ctx context.Context, texts []string, model string) ([]providers.EmbeddingResult, error) {
	m.callCount++
	if m.err != nil {
		return nil, m.err
	}

	results := make([]providers.EmbeddingResult, len(texts))
	for i, text := range texts {
		emb, ok := m.embeddings[text]
		if !ok {
			// default to zero vector if not found in mock
			emb = make([]float32, 3)
		}
		results[i] = providers.EmbeddingResult{
			Embedding: emb,
			Index:     i,
		}
	}
	return results, nil
}

type mockTool struct {
	name string
	desc string
}

func (t *mockTool) Name() string                                                 { return t.name }
func (t *mockTool) Description() string                                          { return t.desc }
func (t *mockTool) Parameters() map[string]any                                   { return nil }
func (t *mockTool) Execute(ctx context.Context, args map[string]any) *ToolResult { return nil }

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float32
	}{
		{
			name:     "identical vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{1, 0, 0},
			expected: 1.0,
		},
		{
			name:     "orthogonal vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{0, 1, 0},
			expected: 0.0,
		},
		{
			name:     "opposite vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{-1, 0, 0},
			expected: -1.0,
		},
		{
			name:     "different lengths",
			a:        []float32{1, 0},
			b:        []float32{1, 0, 0},
			expected: 0.0, // returns 0
		},
		{
			name:     "zero vector",
			a:        []float32{0, 0, 0},
			b:        []float32{1, 1, 1},
			expected: 0.0,
		},
		{
			name:     "similar vectors",
			a:        []float32{1, 1, 0},
			b:        []float32{1, 0, 0},
			expected: float32(1.0 / math.Sqrt(2.0)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cosineSimilarity(tt.a, tt.b)
			// allow tiny floating point difference
			if math.Abs(float64(result-tt.expected)) > 1e-6 {
				t.Errorf("expected %f, got %f", tt.expected, result)
			}
		})
	}
}

func TestSemanticMatcher_MatchTools(t *testing.T) {
	// Setup vector space:
	// "browser": [1, 0, 0]
	// "search":  [0.8, 0, 0] - similar to browser
	// "file":    [0, 1, 0]
	// "read":    [0, 0.8, 0] - similar to file
	// "bash":    [0, 0, 1]
	// "shell":   [0, 0, 0.8] - similar to bash

	mockProv := &mockEmbeddingProvider{
		embeddings: map[string][]float32{
			"search query":          {0.8, 0, 0}, // intent
			"read log file":         {0, 0.8, 0}, // intent
			"run command":           {0, 0, 0.8}, // intent
			"browser: open webpage": {1, 0, 0},
			"file: read file":       {0, 1, 0},
			"bash: execute shell":   {0, 0, 1},
		},
	}

	matcher := NewSemanticMatcher(mockProv, "test-model")

	tools := []Tool{
		&mockTool{name: "browser", desc: "open webpage"},
		&mockTool{name: "file", desc: "read file"},
		&mockTool{name: "bash", desc: "execute shell"},
	}

	tests := []struct {
		name         string
		intent       string
		topK         int
		expectedDeps []string // list of expected tool names in order
	}{
		{
			name:         "match browser",
			intent:       "search query",
			topK:         1,
			expectedDeps: []string{"browser"},
		},
		{
			name:         "match file",
			intent:       "read log file",
			topK:         1,
			expectedDeps: []string{"file"},
		},
		{
			name:         "match bash",
			intent:       "run command",
			topK:         1,
			expectedDeps: []string{"bash"},
		},
		{
			name:         "top 2 picks browser and file", // Since our mock only defines similarity for 1 tool, the others will be zero
			intent:       "search query",
			topK:         2,
			expectedDeps: []string{"browser", "file"}, // order of remaining tools with 0 score is stable or undefined, but browser is definitely first
		},
		{
			name:         "topK larger than tools",
			intent:       "search query",
			topK:         5,
			expectedDeps: []string{"browser", "file", "bash"}, // file and bash tie for 0, so order might depend on slice order, but all 3 should be returned.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := matcher.MatchTools(context.Background(), tt.intent, tools, tt.topK)

			if len(matched) != len(tt.expectedDeps) {
				t.Fatalf("expected %d tools, got %d", len(tt.expectedDeps), len(matched))
			}

			if tt.topK == 1 {
				if matched[0].Name() != tt.expectedDeps[0] {
					t.Errorf("expected %s to be top match, got %s", tt.expectedDeps[0], matched[0].Name())
				}
			} else {
				// verify expected tools are in the result
				for _, exp := range tt.expectedDeps {
					found := false
					for _, m := range matched {
						if m.Name() == exp {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("expected tool %s to be in matched results", exp)
					}
				}
			}
		})
	}
}

func TestSemanticMatcher_Caching(t *testing.T) {
	mockProv := &mockEmbeddingProvider{
		embeddings: map[string][]float32{
			"intent 1":              {1, 0, 0},
			"intent 2":              {0, 1, 0},
			"browser: open webpage": {1, 0, 0},
		},
	}

	matcher := NewSemanticMatcher(mockProv, "test-model")

	tools := []Tool{
		&mockTool{name: "browser", desc: "open webpage"},
		&mockTool{name: "search", desc: "search web"},
	}

	// First match - should embed intent AND tool (1 call, 2 texts)
	_ = matcher.MatchTools(context.Background(), "intent 1", tools, 1)

	if mockProv.callCount != 1 {
		t.Errorf("expected 1 API call, got %d", mockProv.callCount)
	}

	// Second match with new intent - should embed intent ONLY (1 call, 1 text)
	_ = matcher.MatchTools(context.Background(), "intent 2", tools, 1)

	if mockProv.callCount != 2 {
		t.Errorf("expected 2 API calls, got %d", mockProv.callCount)
	}

	// Check that the tool's embedding was cached
	matcher.mu.RLock()
	emb, exists := matcher.cache["browser"]
	matcher.mu.RUnlock()

	if !exists {
		t.Errorf("expected tool to be cached")
	}
	if !reflect.DeepEqual(emb, []float32{1, 0, 0}) {
		t.Errorf("expected embedding to be {1,0,0}, got %v", emb)
	}
}
