package agent

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"

	"github.com/grasberg/sofia/pkg/routing"
)

// delegationThreshold is the minimum score for automatic delegation.
const delegationThreshold = 0.35

// delegationCandidate holds an agent and its delegation score.
type delegationCandidate struct {
	Agent *AgentInstance
	Score float64
}

// swedishNames is a pool of Swedish girls' names used for auto-created subagents.
var swedishNames = []string{
	"Astrid", "Ebba", "Ella", "Elsa", "Freja", "Greta", "Hilda",
	"Ines", "Iris", "Klara", "Lova", "Maja", "Nora", "Saga",
	"Signe", "Sigrid", "Stella", "Svea", "Tyra", "Wilma",
	"Alma", "Alva", "Edith", "Elin", "Elvira", "Emmy", "Hilma",
	"Lykke", "Märta", "Ronja", "Siri", "Tilde", "Tuva", "Vera",
}

// usedNames tracks which names have already been assigned to avoid duplicates.
var (
	usedNames   = map[string]bool{}
	usedNamesMu sync.Mutex
)

// maxUsedNames is the cap on the usedNames map. When exceeded, the map is reset.
const maxUsedNames = 100

func pickSwedishName() string {
	usedNamesMu.Lock()
	defer usedNamesMu.Unlock()

	// Cap: reset when map gets too large
	if len(usedNames) >= maxUsedNames {
		usedNames = map[string]bool{}
	}

	// Try to find an unused name
	shuffled := make([]string, len(swedishNames))
	copy(shuffled, swedishNames)
	rand.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
	for _, n := range shuffled {
		if !usedNames[n] {
			usedNames[n] = true
			return n
		}
	}
	// All names used — pick random with suffix
	base := swedishNames[rand.Intn(len(swedishNames))]
	name := fmt.Sprintf("%s-%d", base, rand.Intn(99)+1)
	usedNames[name] = true
	return name
}

// ResetNames clears the used names set, allowing names to be reused.
func ResetNames() {
	usedNamesMu.Lock()
	defer usedNamesMu.Unlock()
	usedNames = map[string]bool{}
}

// scoreCandidate computes a delegation score in [0,1] for a given sub-agent.
func scoreCandidate(agent *AgentInstance, msgLower string) float64 {
	const (
		wSkills  = 0.60
		wPurpose = 0.25
		wHint    = 0.15
	)

	score := 0.0

	if len(agent.SkillsFilter) > 0 {
		matched := 0
		for _, skill := range agent.SkillsFilter {
			if strings.Contains(msgLower, strings.ToLower(skill)) {
				matched++
			}
		}
		score += wSkills * (float64(matched) / float64(len(agent.SkillsFilter)))
	}

	if agent.PurposePrompt != "" {
		purposeLower := strings.ToLower(agent.PurposePrompt)
		words := strings.Fields(purposeLower)
		significant := make([]string, 0, len(words))
		for _, w := range words {
			w = strings.Trim(w, ".,;:!?\"'()")
			if len(w) > 3 {
				significant = append(significant, w)
			}
		}
		if len(significant) > 0 {
			matched := 0
			for _, w := range significant {
				if strings.Contains(msgLower, w) {
					matched++
				}
			}
			score += wPurpose * (float64(matched) / float64(len(significant)))
		}
	}

	agentNameLower := strings.ToLower(strings.TrimSpace(agent.Name))
	templateLower := strings.ToLower(strings.TrimSpace(agent.Template))
	if agentNameLower != "" && strings.Contains(msgLower, agentNameLower) {
		score += wHint
	} else if templateLower != "" && strings.Contains(msgLower, templateLower) {
		score += wHint
	}

	return score
}

// delegateToAll returns ALL sub-agents that score above delegationThreshold,
// sorted by score descending. Returns nil if none qualify.
func (al *AgentLoop) delegateToAll(msg string) []delegationCandidate {
	msgLower := strings.ToLower(msg)
	var candidates []delegationCandidate

	for _, id := range al.registry.ListAgentIDs() {
		if routing.NormalizeAgentID(id) == routing.DefaultAgentID {
			continue
		}
		agent, ok := al.registry.GetAgent(id)
		if !ok || agent == nil {
			continue
		}
		s := scoreCandidate(agent, msgLower)
		if s >= delegationThreshold {
			candidates = append(candidates, delegationCandidate{Agent: agent, Score: s})
		}
	}

	// Sort by score descending
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].Score > candidates[i].Score {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	if len(candidates) == 0 {
		return nil
	}
	return candidates
}
