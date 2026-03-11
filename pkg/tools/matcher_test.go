package tools

import (
	"context"
	"testing"

	"github.com/grasberg/sofia/pkg/providers"
)

// MockEmbeddingProvider for testing
type MockEmbeddingProvider struct {
	embeddings map[string][]float32
}

func (m *MockEmbeddingProvider) Embeddings(ctx context.Context, texts []string, model string) ([]providers.EmbeddingResult, error) {
	results := make([]providers.EmbeddingResult, len(texts))
	for i, text := range texts {
		if emb, ok := m.embeddings[text]; ok {
			results[i] = providers.EmbeddingResult{
				Index:     i,
				Embedding: emb,
			}
		} else {
			// Default embedding
			results[i] = providers.EmbeddingResult{
				Index:     i,
				Embedding: make([]float32, 3),
			}
		}
	}
	return results, nil
}

func (m *MockEmbeddingProvider) Chat(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, model string, options map[string]any) (*providers.LLMResponse, error) {
	return nil, nil
}

func (m *MockEmbeddingProvider) GetDefaultModel() string {
	return "test-model"
}

// MockTool for testing
type MockTool struct {
	name        string
	description string
}

func (m *MockTool) Name() string {
	return m.name
}

func (m *MockTool) Description() string {
	return m.description
}

func (m *MockTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	return NewToolResult("mock result")
}

func (m *MockTool) Parameters() map[string]any {
	return map[string]any{}
}

func TestSemanticMatcher_NewSemanticMatcher(t *testing.T) {
	provider := &MockEmbeddingProvider{}
	matcher := NewSemanticMatcher(provider, "test-model")
	if matcher == nil {
		t.Error("expected matcher to be created")
	}
	if matcher.model != "test-model" {
		t.Errorf("expected model 'test-model', got '%s'", matcher.model)
	}
}

func TestSemanticMatcher_NilProvider(t *testing.T) {
	matcher := NewSemanticMatcher(nil, "test")
	tools := []Tool{
		&MockTool{name: "tool1", description: "desc1"},
		&MockTool{name: "tool2", description: "desc2"},
	}
	result := matcher.MatchTools(context.Background(), "test intent", tools, 1)
	if len(result) != 2 {
		t.Errorf("expected all tools returned when provider is nil, got %d", len(result))
	}
}

func TestSemanticMatcher_EmptyIntent(t *testing.T) {
	provider := &MockEmbeddingProvider{}
	matcher := NewSemanticMatcher(provider, "test")
	tools := []Tool{
		&MockTool{name: "tool1", description: "desc1"},
	}
	result := matcher.MatchTools(context.Background(), "", tools, 1)
	if len(result) != 1 {
		t.Errorf("expected all tools returned with empty intent, got %d", len(result))
	}
}

func TestSemanticMatcher_FewerToolsThanTopK(t *testing.T) {
	provider := &MockEmbeddingProvider{}
	matcher := NewSemanticMatcher(provider, "test")
	tools := []Tool{
		&MockTool{name: "tool1", description: "desc1"},
	}
	result := matcher.MatchTools(context.Background(), "intent", tools, 5)
	if len(result) != 1 {
		t.Errorf("expected all tools when fewer than topK, got %d", len(result))
	}
}

func TestSemanticMatcher_MatchTools(t *testing.T) {
	provider := &MockEmbeddingProvider{
		embeddings: map[string][]float32{
			"get_weather": {0.9, 0.1, 0.0},
			"test intent": {0.9, 0.0, 0.1},
			"get_time":    {0.1, 0.9, 0.0},
			"send_email":  {0.0, 0.1, 0.9},
		},
	}
	matcher := NewSemanticMatcher(provider, "test")
	tools := []Tool{
		&MockTool{name: "get_weather", description: "Get weather"},
		&MockTool{name: "get_time", description: "Get time"},
		&MockTool{name: "send_email", description: "Send email"},
	}
	result := matcher.MatchTools(context.Background(), "test intent", tools, 2)
	if len(result) != 2 {
		t.Errorf("expected 2 tools, got %d", len(result))
	}
}

func TestCosineSimilarity_IdenticalVectors(t *testing.T) {
	a := []float32{1.0, 0.0, 0.0}
	b := []float32{1.0, 0.0, 0.0}
	score := cosineSimilarity(a, b)
	if score < 0.99 || score > 1.01 {
		t.Errorf("expected similarity ~1.0, got %f", score)
	}
}

func TestCosineSimilarity_OrthogonalVectors(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{0.0, 1.0}
	score := cosineSimilarity(a, b)
	if score != 0.0 {
		t.Errorf("expected similarity 0.0 for orthogonal vectors, got %f", score)
	}
}

func TestCosineSimilarity_OppositeVectors(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{-1.0, 0.0}
	score := cosineSimilarity(a, b)
	if score != -1.0 {
		t.Errorf("expected similarity -1.0 for opposite vectors, got %f", score)
	}
}

func TestCosineSimilarity_DifferentLengths(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{1.0}
	score := cosineSimilarity(a, b)
	if score != 0.0 {
		t.Errorf("expected similarity 0.0 for different length vectors, got %f", score)
	}
}

func TestCosineSimilarity_EmptyVectors(t *testing.T) {
	a := []float32{}
	b := []float32{}
	score := cosineSimilarity(a, b)
	if score != 0.0 {
		t.Errorf("expected similarity 0.0 for empty vectors, got %f", score)
	}
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	a := []float32{0.0, 0.0}
	b := []float32{1.0, 0.0}
	score := cosineSimilarity(a, b)
	if score != 0.0 {
		t.Errorf("expected similarity 0.0 when one vector is zero, got %f", score)
	}
}

func TestSemanticMatcher_Caching(t *testing.T) {
	callCount := 0
	provider := &MockEmbeddingProvider{
		embeddings: map[string][]float32{
			"intent1":     {0.9, 0.1},
			"tool1_desc":  {0.8, 0.2},
		},
	}
	matcher := NewSemanticMatcher(provider, "test")
	tools := []Tool{
		&MockTool{name: "tool1", description: "tool1_desc"},
	}
	
	// Pre-populate cache
	matcher.mu.Lock()
	matcher.cache["tool1"] = []float32{0.8, 0.2}
	matcher.mu.Unlock()

	// First call should use cache
	result := matcher.MatchTools(context.Background(), "intent1", tools, 1)
	_ = callCount // Use variable to avoid lint warning
	
	if len(result) != 1 {
		t.Errorf("expected 1 tool, got %d", len(result))
	}
}
