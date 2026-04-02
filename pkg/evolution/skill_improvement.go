package evolution

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
)

// SkillImprovement represents a suggested improvement to a skill.
type SkillImprovement struct {
	SkillName  string    `json:"skill_name"`
	Issue      string    `json:"issue"`
	Suggestion string    `json:"suggestion"`
	Priority   int       `json:"priority"` // 1-5, 5 being highest
	SourceTask string    `json:"source_task"`
	Timestamp  time.Time `json:"timestamp"`
}

// SkillAnalyzer analyzes agent performance and suggests skill improvements.
type SkillAnalyzer struct {
	db       *memory.MemoryDB
	agentID  string
	provider providers.LLMProvider
	modelID  string
}

// NewSkillAnalyzer creates a new SkillAnalyzer.
func NewSkillAnalyzer(
	db *memory.MemoryDB,
	agentID string,
	provider providers.LLMProvider,
	modelID string,
) *SkillAnalyzer {
	return &SkillAnalyzer{
		db:       db,
		agentID:  agentID,
		provider: provider,
		modelID:  modelID,
	}
}

// AnalyzeAndSuggestImprovements examines recent reflections and suggests skill improvements.
func (sa *SkillAnalyzer) AnalyzeAndSuggestImprovements(ctx context.Context, limit int) ([]SkillImprovement, error) {
	if sa.db == nil || sa.provider == nil {
		return nil, nil
	}

	// Get recent reflections with low scores
	reflections, err := sa.db.GetRecentReflections(sa.agentID, limit)
	if err != nil || len(reflections) == 0 {
		return nil, nil
	}

	// Filter for reflections with issues (score < 0.7 or has WhatFailed)
	var problematic []string
	hasIssues := false
	for _, r := range reflections {
		if r.Score < 0.7 || r.WhatFailed != "" {
			hasIssues = true
			entry := fmt.Sprintf("Task: %s\nWhat failed: %s\nLesson: %s\nScore: %.1f",
				r.TaskSummary, r.WhatFailed, r.Lessons, r.Score)
			problematic = append(problematic, entry)
		}
	}

	if !hasIssues {
		return nil, nil
	}

	prompt := fmt.Sprintf(
		`Analyze these failed or suboptimal task executions and identify patterns that suggest skill gaps.

Failed/Poor Executions:
%s

For each distinct issue, suggest how a skill could be improved or created to prevent this failure in the future.

Respond in this exact JSON format (no markdown, code fences, or extra text):
[
  {
    "issue": "What went wrong or was missing",
    "skill_name": "name of skill to improve or create (lowercase with hyphens)",
    "suggestion": "specific improvement to add to the skill",
    "priority": 1-5 based on frequency and severity
  }
]

Focus on actionable skill improvements. If no clear skill improvements emerge, respond with "[]".`,
		strings.Join(problematic, "\n\n"),
	)

	messages := []providers.Message{
		{Role: "user", Content: prompt},
	}

	resp, err := sa.provider.Chat(ctx, messages, nil, sa.modelID, map[string]any{
		"max_tokens":  1500,
		"temperature": 0.3,
	})
	if err != nil || resp.Content == "" {
		return nil, err
	}

	// Parse JSON response
	content := strings.TrimSpace(resp.Content)
	// Strip markdown if present
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	if content == "[]" || content == "" {
		return nil, nil
	}

	// Simple JSON parsing - try to extract each improvement
	lines := strings.Split(content, "\n")
	var current map[string]string
	var results []SkillImprovement

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, `"issue"`) {
			current = make(map[string]string)
			extractJSONField(line, "issue", current)
		} else if strings.Contains(line, `"skill_name"`) {
			extractJSONField(line, "skill_name", current)
		} else if strings.Contains(line, `"suggestion"`) {
			extractJSONField(line, "suggestion", current)
		} else if strings.Contains(line, `"priority"`) {
			extractJSONField(line, "priority", current)
			// Completed - add to results
			if current != nil && current["issue"] != "" && current["skill_name"] != "" {
				priority := 3 // default
				if current["priority"] != "" {
					fmt.Sscanf(current["priority"], "%d", &priority)
				}
				results = append(results, SkillImprovement{
					SkillName:  current["skill_name"],
					Issue:      current["issue"],
					Suggestion: current["suggestion"],
					Priority:   priority,
					Timestamp:  time.Now(),
				})
			}
		}
	}

	// Fallback: if no improvements parsed, try to parse the whole thing
	if len(results) == 0 && strings.Contains(content, "[") {
		// Try direct unmarshal
		_ = json.Unmarshal([]byte(content), &results)
	}

	return results, nil
}

func extractJSONField(line string, key string, target map[string]string) {
	// Simple extraction of "key": "value" or "key": value
	idx := strings.Index(line, `"`+key+`"`)
	if idx == -1 {
		return
	}
	rest := line[idx+len(key)+3:]
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, ":") {
		rest = strings.TrimPrefix(rest, ":")
		rest = strings.TrimSpace(rest)
	}
	// Get value
	if strings.HasPrefix(rest, `"`) {
		end := strings.Index(rest[1:], `"`)
		if end > 0 {
			target[key] = rest[1 : end+1]
		}
	} else {
		// Number or boolean
		end := strings.IndexAny(rest, ",}")
		if end > 0 {
			target[key] = strings.TrimSpace(rest[:end])
		}
	}
}

// SaveImprovement stores a skill improvement suggestion in the knowledge graph.
func (sa *SkillAnalyzer) SaveImprovement(imp SkillImprovement) error {
	if sa.db == nil {
		return nil
	}
	// Store as a knowledge graph node
	props := fmt.Sprintf(`{"issue": %q, "suggestion": %q, "priority": %d}`,
		imp.Issue, imp.Suggestion, imp.Priority)
	_, err := sa.db.UpsertNode(sa.agentID, "skill_improvement", imp.SkillName, props)
	return err
}

// SkillImprovementPrompts generates skill content improvements using LLM.
type SkillImprovementPrompts struct {
	provider providers.LLMProvider
	modelID  string
}

// NewSkillImprovementPrompts creates a new SkillImprovementPrompts.
func NewSkillImprovementPrompts(provider providers.LLMProvider, modelID string) *SkillImprovementPrompts {
	return &SkillImprovementPrompts{
		provider: provider,
		modelID:  modelID,
	}
}

// GenerateSkillImprovement creates improved skill content based on the improvement suggestion.
func (sip *SkillImprovementPrompts) GenerateSkillImprovement(
	ctx context.Context,
	existingContent, improvement Suggestion,
) (string, error) {
	prompt := fmt.Sprintf(`Improve this skill based on the following improvement suggestion.

Existing Skill Content:
---
%s
---

Improvement Suggestion:
- Issue: %s
- Suggestion: %s

Generate improved skill content that addresses this issue. Keep the same format (Markdown with YAML frontmatter). Maintain the skill's core purpose while adding the improvement.

Respond ONLY with the improved skill content (no explanations, no markdown code fences).`, existingContent, improvement.Issue, improvement.Suggestion)

	messages := []providers.Message{
		{Role: "user", Content: prompt},
	}

	resp, err := sip.provider.Chat(ctx, messages, nil, sip.modelID, map[string]any{
		"max_tokens":  4000,
		"temperature": 0.4,
	})
	if err != nil {
		return "", err
	}

	content := strings.TrimSpace(resp.Content)
	// Strip markdown code fences if present
	content = strings.TrimPrefix(content, "```markdown")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	return strings.TrimSpace(content), nil
}

// Suggestion is a simple struct for skill improvement suggestions
type Suggestion struct {
	Issue      string
	Suggestion string
}
