package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// makeTestAgent builds a minimal AgentInstance for delegation scoring.
func makeTestAgent(id, name, tmpl string, skills []string, purpose string) *AgentInstance {
	return &AgentInstance{
		ID:            id,
		Name:          name,
		Template:      tmpl,
		SkillsFilter:  skills,
		PurposePrompt: purpose,
	}
}

func TestScoreCandidate_SkillsMatch(t *testing.T) {
	agent := makeTestAgent("coder", "Coder", "", []string{"code", "python", "debug"}, "")
	// All three skills present → skills component = 0.60 * 1.0 = 0.60
	score := scoreCandidate(agent, "can you debug my python code please")
	assert.InDelta(t, 0.60, score, 0.01, "full skills match should yield ~0.60")
}

func TestScoreCandidate_PartialSkillsMatch(t *testing.T) {
	agent := makeTestAgent("coder", "Coder", "", []string{"code", "python", "debug"}, "")
	// One of three skills → 0.60 * (1/3) ≈ 0.20
	score := scoreCandidate(agent, "how do I write a loop in python")
	assert.InDelta(t, 0.20, score, 0.01, "1/3 skills match should yield ~0.20")
}

func TestScoreCandidate_NoSkillsButPurpose(t *testing.T) {
	agent := makeTestAgent("researcher", "Researcher", "", nil,
		"Handles research queries about science and history.")
	score := scoreCandidate(agent, "can you do research on science topics")
	assert.Greater(t, score, 0.0, "purpose overlap should yield a positive score")
}

func TestScoreCandidate_NameHint(t *testing.T) {
	agent := makeTestAgent("johanna", "Johanna", "", nil, "")
	score := scoreCandidate(agent, "ask johanna to write a report")
	assert.InDelta(t, 0.15, score, 0.01, "name mention should yield ~0.15")
}

func TestScoreCandidate_TemplateHint(t *testing.T) {
	agent := makeTestAgent("researcher", "Researcher", "research-assistant", nil, "")
	score := scoreCandidate(agent, "use research-assistant for this task")
	assert.InDelta(t, 0.15, score, 0.01, "template mention should yield ~0.15")
}

func TestScoreCandidate_NoMatch(t *testing.T) {
	agent := makeTestAgent("coder", "Coder", "", []string{"code", "python"}, "Write and debug code.")
	score := scoreCandidate(agent, "what is the weather today")
	assert.Less(t, score, delegationThreshold, "unrelated message should score below threshold")
}

// --- delegateToAll integration tests ---

func newLoopWithAgents(agents map[string]*AgentInstance) *AgentLoop {
	r := &AgentRegistry{
		agents: agents,
	}
	return &AgentLoop{registry: r}
}

func TestDelegateToAll_HighScore_Delegates(t *testing.T) {
	main := makeTestAgent("main", "Sofia", "", nil, "")
	coder := makeTestAgent("coder", "Coder", "", []string{"code", "python", "debug", "programming"}, "")

	al := newLoopWithAgents(map[string]*AgentInstance{
		"main":  main,
		"coder": coder,
	})

	// All four skills present → score = 0.60 >= 0.35 ✓
	result := al.delegateToAll("help me debug python programming code")
	assert.NotEmpty(t, result, "should delegate when score >= threshold")
	assert.Equal(t, "coder", result[0].Agent.ID)
}

func TestDelegateToAll_LowScore_ReturnsNil(t *testing.T) {
	main := makeTestAgent("main", "Sofia", "", nil, "")
	coder := makeTestAgent("coder", "Coder", "", []string{"code", "python"}, "")

	al := newLoopWithAgents(map[string]*AgentInstance{
		"main":  main,
		"coder": coder,
	})

	result := al.delegateToAll("what is the weather today")
	assert.Nil(t, result, "unrelated message should not delegate")
}

func TestDelegateToAll_MainAgentNeverSelected(t *testing.T) {
	main := makeTestAgent("main", "Sofia", "", []string{"help", "general", "chat", "tasks"}, "")

	al := newLoopWithAgents(map[string]*AgentInstance{
		"main": main,
	})

	result := al.delegateToAll("help me with general chat tasks please sofia")
	assert.Nil(t, result, "main agent must never be returned by delegateToAll")
}

func TestDelegateToAll_MultipleAgentsSelected(t *testing.T) {
	main := makeTestAgent("main", "Sofia", "", nil, "")
	coder := makeTestAgent("coder", "Coder", "", []string{"code", "python", "debug"}, "")
	researcher := makeTestAgent("researcher", "Researcher", "",
		[]string{"research", "analysis", "data", "science"}, "")

	al := newLoopWithAgents(map[string]*AgentInstance{
		"main":       main,
		"coder":      coder,
		"researcher": researcher,
	})

	// Message touches both agents' skills
	result := al.delegateToAll("research the code for data analysis and debug the python science module")
	assert.Len(t, result, 2, "both agents should be selected")

	ids := []string{result[0].Agent.ID, result[1].Agent.ID}
	assert.Contains(t, ids, "coder")
	assert.Contains(t, ids, "researcher")
}

func TestDelegateToAll_SortedByScore(t *testing.T) {
	main := makeTestAgent("main", "Sofia", "", nil, "")
	coder := makeTestAgent("coder", "Coder", "", []string{"code"}, "")
	researcher := makeTestAgent("researcher", "Researcher", "",
		[]string{"research", "analysis", "data", "science"}, "")

	al := newLoopWithAgents(map[string]*AgentInstance{
		"main":       main,
		"coder":      coder,
		"researcher": researcher,
	})

	result := al.delegateToAll("do research and analysis on science data, ask researcher")
	assert.NotEmpty(t, result)
	assert.Equal(t, "researcher", result[0].Agent.ID, "highest scoring agent should be first")
}

func TestPickSwedishName(t *testing.T) {
	name := pickSwedishName()
	assert.NotEmpty(t, name)
	// Should be unique on repeated calls
	name2 := pickSwedishName()
	assert.NotEqual(t, name, name2)
}
