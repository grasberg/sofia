package tools

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/providers"
)

type SemanticMatcher struct {
	provider providers.EmbeddingProvider
	model    string
	cache    map[string][]float32
	mu       sync.RWMutex
}

func NewSemanticMatcher(provider providers.EmbeddingProvider, model string) *SemanticMatcher {
	return &SemanticMatcher{
		provider: provider,
		model:    model,
		cache:    make(map[string][]float32),
	}
}

// matchScore represents the similarity score for a tool.
type matchScore struct {
	tool  Tool
	score float32
}

// MatchTools returns the topK tools that best fit the given user intent string.
// If provider is nil, or errors occur, it gracefully degrades by returning all tools.
func (sm *SemanticMatcher) MatchTools(
	ctx context.Context,
	intent string,
	tools []Tool,
	topK int,
) []Tool {
	if sm == nil || sm.provider == nil || len(tools) <= topK || intent == "" {
		return tools
	}

	start := time.Now()

	// Build a list of texts to embed. Index 0 is the intent.
	// Indices 1..N are the tools that are NOT in the cache.
	var textsToEmbed []string
	textsToEmbed = append(textsToEmbed, intent)

	toolToEmbedIndex := make(map[string]int) // Maps tool name -> index in textsToEmbed
	for _, t := range tools {
		sm.mu.RLock()
		_, exists := sm.cache[t.Name()]
		sm.mu.RUnlock()

		if !exists {
			desc := t.Description()
			if desc == "" {
				desc = t.Name() // Fallback if no description
			}
			toolStr := fmt.Sprintf("%s: %s", t.Name(), desc)
			toolToEmbedIndex[t.Name()] = len(textsToEmbed)
			textsToEmbed = append(textsToEmbed, toolStr)
		}
	}

	// Call provider for embeddings
	results, err := sm.provider.Embeddings(ctx, textsToEmbed, sm.model)
	if err != nil {
		logger.WarnCF("semantic_matcher", "Failed to rank tools, falling back to all tools", map[string]any{
			"error": err.Error(),
			"count": len(tools),
		})
		return tools
	}

	if len(results) != len(textsToEmbed) {
		logger.WarnCF("semantic_matcher", "Embeddings length mismatch, falling back to all tools", map[string]any{
			"expected": len(textsToEmbed),
			"got":      len(results),
		})
		return tools
	}

	// Extract intent embedding (always index 0)
	var intentEmbedding []float32
	for _, r := range results {
		if r.Index == 0 {
			intentEmbedding = r.Embedding
			break
		}
	}

	// Cache new tool embeddings
	sm.mu.Lock()
	for _, t := range tools {
		if idx, isNew := toolToEmbedIndex[t.Name()]; isNew {
			for _, r := range results {
				if r.Index == idx {
					sm.cache[t.Name()] = r.Embedding
					break
				}
			}
		}
	}
	sm.mu.Unlock()

	// Calculate similarities
	scores := make([]matchScore, 0, len(tools))
	for _, t := range tools {
		sm.mu.RLock()
		toolEmbed := sm.cache[t.Name()]
		sm.mu.RUnlock()

		if len(toolEmbed) == 0 {
			continue // Should never happen unless caching failed
		}

		score := cosineSimilarity(intentEmbedding, toolEmbed)
		scores = append(scores, matchScore{tool: t, score: score})
	}

	// Sort by score descending
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})

	// Pick top K
	var matched []Tool
	for i := 0; i < topK && i < len(scores); i++ {
		matched = append(matched, scores[i].tool)
	}

	logger.DebugCF("semantic_matcher", "Matched tools", map[string]any{
		"intent":        strings.TrimSpace(intent),
		"filtered_from": len(tools),
		"filtered_to":   len(matched),
		"duration_ms":   time.Since(start).Milliseconds(),
	})

	return matched
}

// cosineSimilarity calculates the cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float32
	for i := 0; i < len(a); i++ {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return float32(float64(dotProduct) / (math.Sqrt(float64(normA)) * math.Sqrt(float64(normB))))
}
