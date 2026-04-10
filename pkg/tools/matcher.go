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
	tracker  *ToolTracker // optional usage stats for ranking
	disabled bool         // set after first embedding failure to stop retrying

	// Intent embedding cache to avoid redundant API calls
	intentCache map[string][]float32 // key: intent hash → embedding
}

func NewSemanticMatcher(provider providers.EmbeddingProvider, model string) *SemanticMatcher {
	return &SemanticMatcher{
		provider:    provider,
		model:       model,
		cache:       make(map[string][]float32),
		intentCache: make(map[string][]float32),
	}
}

// SetTracker attaches a ToolTracker for usage-based ranking.
func (sm *SemanticMatcher) SetTracker(tracker *ToolTracker) {
	sm.tracker = tracker
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
	if sm == nil || sm.provider == nil || sm.disabled || len(tools) <= topK || intent == "" {
		return tools
	}

	start := time.Now()

	// Check intent cache to avoid redundant API calls
	intentHash := hashIntent(intent)
	sm.mu.RLock()
	intentEmbedding, intentCached := sm.intentCache[intentHash]
	sm.mu.RUnlock()

	// Build a list of texts to embed. Index 0 is the intent (if not cached).
	// Indices 1..N are the tools that are NOT in the cache.
	var textsToEmbed []string
	if !intentCached {
		textsToEmbed = append(textsToEmbed, intent)
	}

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

	// Call provider for embeddings (only if needed)
	var results []providers.EmbeddingResult
	var err error
	if len(textsToEmbed) > 0 {
		results, err = sm.provider.Embeddings(ctx, textsToEmbed, sm.model)
		if err != nil {
			logger.WarnCF("semantic_matcher", "Embeddings failed, disabling semantic matching (using keyword fallback)", map[string]any{
				"error": err.Error(),
				"model": sm.model,
			})
			sm.mu.Lock()
			sm.disabled = true
			sm.mu.Unlock()
			return tools
		}

		if len(results) != len(textsToEmbed) {
			logger.WarnCF("semantic_matcher", "Embeddings returned no data, disabling semantic matching (using keyword fallback). "+
				"This usually means the provider does not support the embedding model.", map[string]any{
				"expected": len(textsToEmbed),
				"got":      len(results),
				"model":    sm.model,
			})
			sm.mu.Lock()
			sm.disabled = true
			sm.mu.Unlock()
			return tools
		}
	}

	// Cache intent embedding if it was embedded
	if !intentCached && len(results) > 0 {
		for _, r := range results {
			if r.Index == 0 {
				sm.mu.Lock()
				sm.intentCache[intentHash] = r.Embedding
				sm.mu.Unlock()
				break
			}
		}
	}

	// Extract intent embedding (from cache or results)
	if intentCached {
		intentEmbedding = intentEmbedding
	} else {
		for _, r := range results {
			if r.Index == 0 {
				intentEmbedding = r.Embedding
				break
			}
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

		// Apply usage-based ranking boost
		if sm.tracker != nil {
			if stats, ok := sm.tracker.GetStat(t.Name()); ok {
				// Boost for high success rate (0-0.3 boost)
				successBoost := float32(stats.SuccessRate) * 0.3

				// Boost for low latency (0-0.2 boost)
				avgLatencyMs := stats.AverageTime.Milliseconds()
				latencyBoost := float32(max(0, 1000-int(avgLatencyMs))) / 1000.0 * 0.2

				score += score * (successBoost + latencyBoost)
			}
		}

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

// coreTools are always included regardless of keyword match score.
// NOTE: "message" is intentionally excluded — small models misuse it for
// conversational replies instead of returning text directly.
var coreTools = map[string]bool{
	ToolExec:  true,
	ToolShell: true,
}

// max returns the maximum of two integers.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// hashIntent creates a simple hash of the intent string for caching.
func hashIntent(intent string) string {
	// Simple hash: sum of runes modulo a prime number
	var hash int
	for _, r := range intent {
		hash = (hash*31 + int(r)) % 1000000007
	}
	return fmt.Sprintf("intent_%d", hash)
}

// KeywordMatchTools returns the topK tools whose name or description best
// matches the user intent, using simple keyword overlap. No API calls needed.
// Core tools (message, exec) are always included.
func KeywordMatchTools(intent string, allTools []Tool, topK int) []Tool {
	if len(allTools) <= topK || intent == "" {
		return allTools
	}

	intentWords := tokenize(intent)
	if len(intentWords) == 0 {
		return allTools
	}

	type scored struct {
		tool  Tool
		score int
		core  bool
	}

	var results []scored
	for _, t := range allTools {
		if coreTools[t.Name()] {
			results = append(results, scored{tool: t, core: true})
			continue
		}
		text := strings.ToLower(t.Name() + " " + t.Description())
		toolWords := tokenize(text)
		score := 0
		for _, iw := range intentWords {
			for _, tw := range toolWords {
				if tw == iw || strings.Contains(tw, iw) || strings.Contains(iw, tw) {
					score++
				}
			}
		}
		results = append(results, scored{tool: t, score: score})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].core != results[j].core {
			return results[i].core
		}
		return results[i].score > results[j].score
	})

	matched := make([]Tool, 0, topK)
	for i := 0; i < len(results) && len(matched) < topK; i++ {
		matched = append(matched, results[i].tool)
	}
	return matched
}

func tokenize(s string) []string {
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r >= 0x80)
	})
	// Deduplicate and skip very short words
	seen := make(map[string]bool, len(words))
	var out []string
	for _, w := range words {
		if len(w) >= 3 && !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	return out
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
