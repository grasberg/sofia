package eval

import "context"

// JudgeFunc calls an LLM to evaluate a response. Returns a score 0.0-1.0.
// The eval package never imports provider packages — callers inject the
// implementation.
type JudgeFunc func(ctx context.Context, input, output, criteria string) (float64, error)

// JudgeResult holds the LLM judge's assessment.
type JudgeResult struct {
	Score  float64 `json:"score"`
	Reason string  `json:"reason"`
}

// DefaultJudgePrompt constructs a prompt asking an LLM to rate a response
// on a 0.0-1.0 scale and provide reasoning.
const DefaultJudgePrompt = `You are an evaluation judge. Score the following response on a scale from 0.0 to 1.0.

Input given to the agent:
{{INPUT}}

Agent response:
{{OUTPUT}}

Evaluation criteria:
{{CRITERIA}}

Respond with ONLY a JSON object in this exact format (no other text):
{"score": <float between 0.0 and 1.0>, "reason": "<brief explanation>"}
`
