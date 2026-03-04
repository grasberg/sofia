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
	// "research", "science" both present in message → 2/5 significant words ≈ 0.10
	score := scoreCandidate(agent, "can you do research on science topics")
	assert.Greater(t, score, 0.0, "purpose overlap should yield a positive score")
}

func TestScoreCandidate_NameHint(t *testing.T) {
	agent := makeTestAgent("johanna", "Johanna", "", nil, "")
	// Name appears verbatim → hints component = 0.15
	score := scoreCandidate(agent, "ask johanna to write a report")
	assert.InDelta(t, 0.15, score, 0.01, "name mention should yield ~0.15")
}

func TestScoreCandidate_TemplateHint(t *testing.T) {
	agent := makeTestAgent("researcher", "Researcher", "research-assistant", nil, "")
	// Template slug present in message
	score := scoreCandidate(agent, "use research-assistant for this task")
	assert.InDelta(t, 0.15, score, 0.01, "template mention should yield ~0.15")
}

func TestScoreCandidate_NoMatch(t *testing.T) {
	agent := makeTestAgent("coder", "Coder", "", []string{"code", "python"}, "Write and debug code.")
	score := scoreCandidate(agent, "what is the weather today")
	assert.Less(t, score, delegationThreshold, "unrelated message should score below threshold")
}

// --- delegateTo integration tests ---

// stubRegistry implements the minimum registry surface needed by delegateTo.
// remove stubRegistry entirely

// agentLister is the interface delegateTo uses on al.registry.
// We satisfy it with stubRegistry via an adapter AgentLoop.
func newLoopWithAgents(agents map[string]*AgentInstance) *AgentLoop {
	r := &AgentRegistry{
		agents: agents,
	}
	return &AgentLoop{registry: r}
}

func TestDelegateTo_HighScore_Delegates(t *testing.T) {
	main := makeTestAgent("main", "Sofia", "", nil, "")
	coder := makeTestAgent("coder", "Coder", "", []string{"code", "python", "debug", "programming"}, "")

	al := newLoopWithAgents(map[string]*AgentInstance{
		"main":  main,
		"coder": coder,
	})

	// All four skills present → score = 0.60 >= 0.72? No — 1.0 * 0.60 = 0.60, still below.
	// Add name mention to push over: 0.60 + 0.15 = 0.75 >= 0.72 ✓
	result := al.delegateTo("help me debug python programming code, ask coder")
	assert.NotNil(t, result, "should delegate when score >= threshold")
	assert.Equal(t, "coder", result.ID)
}

func TestDelegateTo_LowScore_ReturnsNil(t *testing.T) {
	main := makeTestAgent("main", "Sofia", "", nil, "")
	coder := makeTestAgent("coder", "Coder", "", []string{"code", "python"}, "")

	al := newLoopWithAgents(map[string]*AgentInstance{
		"main":  main,
		"coder": coder,
	})

	result := al.delegateTo("what is the weather today")
	assert.Nil(t, result, "unrelated message should not delegate")
}

func TestDelegateTo_MainAgentNeverSelected(t *testing.T) {
	// Even if main scores high (e.g. name in message), delegateTo must skip it.
	main := makeTestAgent("main", "Sofia", "", []string{"help", "general", "chat", "tasks"}, "")

	al := newLoopWithAgents(map[string]*AgentInstance{
		"main": main,
	})

	result := al.delegateTo("help me with general chat tasks please sofia")
	assert.Nil(t, result, "main agent must never be returned by delegateTo")
}

func TestDelegateTo_BestCandidateWins(t *testing.T) {
	main := makeTestAgent("main", "Sofia", "", nil, "")
	coder := makeTestAgent("coder", "Coder", "", []string{"code"}, "")
	// researcher has more skills matching the message
	researcher := makeTestAgent("researcher", "Researcher", "",
		[]string{"research", "analysis", "data", "science"}, "")

	al := newLoopWithAgents(map[string]*AgentInstance{
		"main":       main,
		"coder":      coder,
		"researcher": researcher,
	})

	result := al.delegateTo("do research and analysis on science data, ask researcher")
	assert.NotNil(t, result)
	assert.Equal(t, "researcher", result.ID, "highest scoring agent should win")
}
