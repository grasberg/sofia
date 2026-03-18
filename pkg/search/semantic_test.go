package search

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCosineSimilarity_Identical(t *testing.T) {
	a := []float32{1.0, 2.0, 3.0}
	b := []float32{1.0, 2.0, 3.0}
	sim := CosineSimilarity(a, b)
	assert.InDelta(t, 1.0, sim, 1e-6, "identical vectors should have cosine similarity of 1.0")
}

func TestCosineSimilarity_Orthogonal(t *testing.T) {
	a := []float32{1.0, 0.0, 0.0}
	b := []float32{0.0, 1.0, 0.0}
	sim := CosineSimilarity(a, b)
	assert.InDelta(t, 0.0, sim, 1e-6, "orthogonal vectors should have cosine similarity of 0.0")
}

func TestCosineSimilarity_Empty(t *testing.T) {
	assert.Equal(t, 0.0, CosineSimilarity(nil, nil), "nil vectors should return 0")
	assert.Equal(t, 0.0, CosineSimilarity([]float32{}, []float32{}), "empty vectors should return 0")
	assert.Equal(t, 0.0, CosineSimilarity([]float32{1}, []float32{1, 2}),
		"different length vectors should return 0")
	assert.Equal(t, 0.0, CosineSimilarity([]float32{0, 0}, []float32{1, 2}),
		"zero-norm vector should return 0")
}

func TestKeywordSearch_BasicMatch(t *testing.T) {
	messages := []MessageEntry{
		{SessionKey: "s1", Content: "The weather today is sunny", Role: "assistant"},
		{SessionKey: "s1", Content: "Tell me about the weather", Role: "user"},
		{SessionKey: "s2", Content: "I like programming in Go", Role: "user"},
	}

	results := KeywordSearch("weather", messages, 10)
	require.Len(t, results, 2)
	assert.Equal(t, 1.0, results[0].Score)
	assert.Equal(t, 1.0, results[1].Score)

	// Both weather-related messages should be returned
	contents := map[string]bool{results[0].Content: true, results[1].Content: true}
	assert.True(t, contents["The weather today is sunny"])
	assert.True(t, contents["Tell me about the weather"])
}

func TestKeywordSearch_MultipleWords(t *testing.T) {
	messages := []MessageEntry{
		{SessionKey: "s1", Content: "Go is great for building web servers", Role: "assistant"},
		{SessionKey: "s2", Content: "Python is great for data science", Role: "assistant"},
		{SessionKey: "s3", Content: "Go and Python are both popular", Role: "user"},
	}

	results := KeywordSearch("go python", messages, 10)
	require.Len(t, results, 3)

	// s3 matches both words and should score highest
	assert.Equal(t, "s3", results[0].SessionKey)
	assert.InDelta(t, 1.0, results[0].Score, 1e-6, "message matching all query words should score 1.0")

	// s1 and s2 each match one word
	assert.InDelta(t, 0.5, results[1].Score, 1e-6)
	assert.InDelta(t, 0.5, results[2].Score, 1e-6)
}

func TestKeywordSearch_NoMatch(t *testing.T) {
	messages := []MessageEntry{
		{SessionKey: "s1", Content: "Hello world", Role: "user"},
		{SessionKey: "s2", Content: "Goodbye world", Role: "assistant"},
	}

	results := KeywordSearch("kubernetes", messages, 10)
	assert.Empty(t, results)
}

func TestKeywordSearch_TopK(t *testing.T) {
	messages := []MessageEntry{
		{SessionKey: "s1", Content: "Go language basics", Role: "user"},
		{SessionKey: "s2", Content: "Go concurrency patterns", Role: "user"},
		{SessionKey: "s3", Content: "Go error handling", Role: "user"},
		{SessionKey: "s4", Content: "Go testing techniques", Role: "user"},
		{SessionKey: "s5", Content: "Go web frameworks", Role: "user"},
	}

	results := KeywordSearch("go", messages, 3)
	require.Len(t, results, 3, "should return at most topK results")

	// All should have the same score since they all match "go"
	for _, r := range results {
		assert.InDelta(t, 1.0, r.Score, 1e-6)
	}
}

func TestKeywordSearch_EmptyQuery(t *testing.T) {
	messages := []MessageEntry{
		{SessionKey: "s1", Content: "Hello world", Role: "user"},
	}
	results := KeywordSearch("", messages, 10)
	assert.Empty(t, results)
}

func TestKeywordSearch_CaseInsensitive(t *testing.T) {
	messages := []MessageEntry{
		{SessionKey: "s1", Content: "GOLANG is Great", Role: "user"},
	}
	results := KeywordSearch("golang great", messages, 10)
	require.Len(t, results, 1)
	assert.InDelta(t, 1.0, results[0].Score, 1e-6)
}

func TestKeywordSearch_PreservesTimestamp(t *testing.T) {
	messages := []MessageEntry{
		{SessionKey: "s1", Content: "test message", Role: "user", Timestamp: "2026-03-18T10:00:00Z"},
	}
	results := KeywordSearch("test", messages, 10)
	require.Len(t, results, 1)
	assert.Equal(t, "2026-03-18T10:00:00Z", results[0].Timestamp)
}
