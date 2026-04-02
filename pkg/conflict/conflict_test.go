package conflict

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectNoConflicts(t *testing.T) {
	outputs := []Output{
		{AgentID: "a1", Content: "The answer is 42"},
		{AgentID: "a2", Content: "The answer is 42"},
	}
	result := Detect(outputs)
	assert.False(t, result.HasConflicts)
	assert.Equal(t, 1.0, result.Agreement)
}

func TestDetectSingleOutput(t *testing.T) {
	result := Detect([]Output{{AgentID: "a1", Content: "hello"}})
	assert.False(t, result.HasConflicts)
	assert.Equal(t, 1.0, result.Agreement)
}

func TestDetectEmpty(t *testing.T) {
	result := Detect(nil)
	assert.False(t, result.HasConflicts)
	assert.Equal(t, 1.0, result.Agreement)
}

func TestDetectContradiction(t *testing.T) {
	outputs := []Output{
		{AgentID: "a1", Content: "Yes, the server is running correctly"},
		{AgentID: "a2", Content: "No, the server is down and not responding"},
	}
	result := Detect(outputs)
	assert.True(t, result.HasConflicts)
	assert.Len(t, result.Conflicts, 1)
	assert.Equal(t, "contradiction", result.Conflicts[0].Type)
	assert.Equal(t, "high", result.Conflicts[0].Severity)
}

func TestDetectNumericContradiction(t *testing.T) {
	outputs := []Output{
		{AgentID: "a1", Content: "The server has 4 CPU cores and 16 GB RAM"},
		{AgentID: "a2", Content: "The server has 8 CPU cores and 32 GB RAM"},
	}
	result := Detect(outputs)
	assert.True(t, result.HasConflicts)
	found := false
	for _, c := range result.Conflicts {
		if c.Type == "contradiction" {
			found = true
			break
		}
	}
	assert.True(t, found, "should detect numeric contradiction")
}

func TestDetectDivergence(t *testing.T) {
	outputs := []Output{
		{AgentID: "a1", Content: "Implemented the login feature using OAuth2 with Google provider"},
		{AgentID: "a2", Content: "Fixed the database migration script for PostgreSQL version upgrade"},
	}
	result := Detect(outputs)
	assert.True(t, result.HasConflicts)
	found := false
	for _, c := range result.Conflicts {
		if c.Type == "divergence" {
			found = true
			break
		}
	}
	assert.True(t, found, "should detect divergence")
}

func TestDetectThreeAgentsPartialAgreement(t *testing.T) {
	outputs := []Output{
		{AgentID: "a1", Content: "The optimal approach is to use a cache layer with Redis"},
		{AgentID: "a2", Content: "The optimal approach is to use a caching layer with Redis for performance"},
		{AgentID: "a3", Content: "We should rewrite the entire service in Rust for better performance"},
	}
	result := Detect(outputs)
	// a1 and a2 agree, a3 diverges
	assert.True(t, result.HasConflicts)
	assert.True(t, result.Agreement > 0.0)
	assert.True(t, result.Agreement < 1.0)
}

// --- Resolution strategy tests ---

func TestResolveMajorityVote(t *testing.T) {
	outputs := []Output{
		{AgentID: "a1", Content: "Use Redis for caching"},
		{AgentID: "a2", Content: "Use Redis for the caching layer"},
		{AgentID: "a3", Content: "Use a completely custom in-memory store built from scratch"},
	}
	res := Resolve(outputs, StrategyMajorityVote)
	assert.Equal(t, "majority_vote", res.Strategy)
	assert.NotNil(t, res.Winner)
	// a1 and a2 are similar, should win
	assert.Contains(t, []string{"a1", "a2"}, res.Winner.AgentID)
	assert.Contains(t, res.Reason, "2/3")
}

func TestResolvePriority(t *testing.T) {
	outputs := []Output{
		{AgentID: "general", Content: "Use any database", Priority: 1},
		{AgentID: "db-expert", Content: "Use PostgreSQL with proper indexing", Priority: 10},
		{AgentID: "intern", Content: "Use SQLite", Priority: 0},
	}
	res := Resolve(outputs, StrategyPriority)
	assert.Equal(t, "priority", res.Strategy)
	assert.NotNil(t, res.Winner)
	assert.Equal(t, "db-expert", res.Winner.AgentID)
	assert.Len(t, res.Rejected, 2)
}

func TestResolvePriorityTiebreakByScore(t *testing.T) {
	outputs := []Output{
		{AgentID: "a1", Content: "Option A", Priority: 5, Score: 0.8},
		{AgentID: "a2", Content: "Option B", Priority: 5, Score: 0.9},
	}
	res := Resolve(outputs, StrategyPriority)
	assert.Equal(t, "a2", res.Winner.AgentID)
}

func TestResolveMerge(t *testing.T) {
	outputs := []Output{
		{AgentID: "a1", Content: "The server uses nginx. It runs on port 80."},
		{AgentID: "a2", Content: "The server uses nginx. The database is PostgreSQL."},
	}
	res := Resolve(outputs, StrategyMerge)
	assert.Equal(t, "merge", res.Strategy)
	assert.Contains(t, res.Merged, "nginx")
	assert.Contains(t, res.Merged, "port 80")
	assert.Contains(t, res.Merged, "PostgreSQL")
}

func TestResolveMergeDeduplication(t *testing.T) {
	outputs := []Output{
		{AgentID: "a1", Content: "The server is healthy. CPU usage is low."},
		{AgentID: "a2", Content: "The server is healthy. Memory usage is normal."},
	}
	res := Resolve(outputs, StrategyMerge)
	// "The server is healthy." should appear only once
	count := strings.Count(strings.ToLower(res.Merged), "the server is healthy")
	assert.Equal(t, 1, count, "duplicate sentence should be merged, got: %s", res.Merged)
}

func TestResolveShortest(t *testing.T) {
	outputs := []Output{
		{AgentID: "a1", Content: "A very long and verbose explanation of the solution"},
		{AgentID: "a2", Content: "Short answer"},
	}
	res := Resolve(outputs, StrategyShortest)
	assert.Equal(t, "shortest", res.Strategy)
	assert.Equal(t, "a2", res.Winner.AgentID)
}

func TestResolveLongest(t *testing.T) {
	outputs := []Output{
		{AgentID: "a1", Content: "Brief"},
		{AgentID: "a2", Content: "A comprehensive and detailed answer with much more information"},
	}
	res := Resolve(outputs, StrategyLongest)
	assert.Equal(t, "longest", res.Strategy)
	assert.Equal(t, "a2", res.Winner.AgentID)
}

func TestResolveAll(t *testing.T) {
	outputs := []Output{
		{AgentID: "a1", Content: "Answer one"},
		{AgentID: "a2", Content: "Answer two"},
	}
	res := Resolve(outputs, StrategyAll)
	assert.Equal(t, "all", res.Strategy)
	assert.Contains(t, res.Merged, "Answer one")
	assert.Contains(t, res.Merged, "Answer two")
	assert.Contains(t, res.Merged, "[Agent a1]")
}

func TestResolveSingleOutput(t *testing.T) {
	outputs := []Output{{AgentID: "a1", Content: "Only one"}}
	res := Resolve(outputs, StrategyMajorityVote)
	assert.NotNil(t, res.Winner)
	assert.Equal(t, "a1", res.Winner.AgentID)
	assert.Contains(t, res.Reason, "single output")
}

func TestResolveEmpty(t *testing.T) {
	res := Resolve(nil, StrategyMajorityVote)
	assert.Contains(t, res.Reason, "no outputs")
}

// --- Helper tests ---

func TestContentSimilarity(t *testing.T) {
	tests := []struct {
		a, b   string
		minSim float64
		maxSim float64
	}{
		{"hello world", "hello world", 1.0, 1.0},
		{"", "", 1.0, 1.0},
		{"foo bar baz", "completely different words", 0.0, 0.1},
		{"the server uses redis", "the server uses redis for caching", 0.5, 0.9},
	}
	for _, tt := range tests {
		sim := contentSimilarity(tt.a, tt.b)
		assert.GreaterOrEqual(t, sim, tt.minSim, "similarity(%q, %q) >= %.2f", tt.a, tt.b, tt.minSim)
		assert.LessOrEqual(t, sim, tt.maxSim, "similarity(%q, %q) <= %.2f", tt.a, tt.b, tt.maxSim)
	}
}

func TestDetectContradictionYesNo(t *testing.T) {
	a := Output{AgentID: "a1", Content: "Yes, it works"}
	b := Output{AgentID: "a2", Content: "No, it does not work"}
	c := detectContradiction(a, b)
	assert.NotNil(t, c)
	assert.Equal(t, "contradiction", c.Type)
}

func TestDetectContradictionNoContradiction(t *testing.T) {
	a := Output{AgentID: "a1", Content: "The server is running"}
	b := Output{AgentID: "a2", Content: "The database is connected"}
	c := detectContradiction(a, b)
	assert.Nil(t, c)
}

func TestGroupBySimilarity(t *testing.T) {
	outputs := []Output{
		{AgentID: "a1", Content: "Use Redis for caching"},
		{AgentID: "a2", Content: "Use Redis for the caching layer"},
		{AgentID: "a3", Content: "Rewrite in Rust"},
	}
	groups := groupBySimilarity(outputs, 0.5)
	assert.Len(t, groups, 2) // Redis group + Rust group
}

func TestFormatDetectResult(t *testing.T) {
	dr := DetectResult{
		HasConflicts: true,
		Conflicts: []Conflict{
			{
				Type: "contradiction", Severity: "high", Description: "test conflict",
				Outputs: []Output{{AgentID: "a1", Content: "yes"}, {AgentID: "a2", Content: "no"}},
			},
		},
		Agreement: 0.5,
	}
	formatted := dr.Format()
	assert.Contains(t, formatted, "1 conflict")
	assert.Contains(t, formatted, "contradiction")
	assert.Contains(t, formatted, "50%")
}

func TestFormatResolution(t *testing.T) {
	winner := Output{AgentID: "a1", Content: "winning answer"}
	res := Resolution{
		Strategy: "majority_vote",
		Winner:   &winner,
		Reason:   "2/3 agents agreed",
		Rejected: []Output{{AgentID: "a2"}},
	}
	formatted := res.Format()
	assert.Contains(t, formatted, "majority_vote")
	assert.Contains(t, formatted, "a1")
	assert.Contains(t, formatted, "a2")
}
