package agent

import "strings"

// ModelPricing holds per-million-token costs in USD.
type ModelPricing struct {
	InputPer1M  float64
	OutputPer1M float64
}

// pricingTable maps model name substrings to pricing.
// Prices are approximate USD per 1M tokens as of early 2026.
var pricingTable = map[string]ModelPricing{
	"gpt-4o":           {InputPer1M: 2.50, OutputPer1M: 10.00},
	"gpt-4o-mini":      {InputPer1M: 0.15, OutputPer1M: 0.60},
	"gpt-4.1":          {InputPer1M: 2.00, OutputPer1M: 8.00},
	"gpt-4.1-mini":     {InputPer1M: 0.40, OutputPer1M: 1.60},
	"gpt-4.1-nano":     {InputPer1M: 0.10, OutputPer1M: 0.40},
	"o3":               {InputPer1M: 2.00, OutputPer1M: 8.00},
	"o3-mini":          {InputPer1M: 1.10, OutputPer1M: 4.40},
	"o4-mini":          {InputPer1M: 1.10, OutputPer1M: 4.40},
	"claude-sonnet":    {InputPer1M: 3.00, OutputPer1M: 15.00},
	"claude-haiku":     {InputPer1M: 0.80, OutputPer1M: 4.00},
	"claude-opus":      {InputPer1M: 15.00, OutputPer1M: 75.00},
	"gemini-2.5-pro":   {InputPer1M: 1.25, OutputPer1M: 10.00},
	"gemini-2.5-flash": {InputPer1M: 0.15, OutputPer1M: 0.60},
	"gemini-2.0-flash": {InputPer1M: 0.10, OutputPer1M: 0.40},
	"deepseek-chat":    {InputPer1M: 0.27, OutputPer1M: 1.10},
	"deepseek-r1":      {InputPer1M: 0.55, OutputPer1M: 2.19},
	"llama":            {InputPer1M: 0.05, OutputPer1M: 0.08},
	"mixtral":          {InputPer1M: 0.24, OutputPer1M: 0.24},
	"qwen":             {InputPer1M: 0.30, OutputPer1M: 0.30},
	"glm-5.1":          {InputPer1M: 1.00, OutputPer1M: 3.20},
	"glm-5":            {InputPer1M: 1.00, OutputPer1M: 3.20},
	"glm-4.7":          {InputPer1M: 0.60, OutputPer1M: 2.20},
	"glm-4.7-flash":    {InputPer1M: 0.00, OutputPer1M: 0.00},
	"glm-4.5-air":      {InputPer1M: 0.20, OutputPer1M: 1.10},
	"glm-4.5":          {InputPer1M: 0.60, OutputPer1M: 2.20},
	"MiniMax-M2.7":     {InputPer1M: 0.15, OutputPer1M: 0.60},
	"MiniMax-M2.5":     {InputPer1M: 0.10, OutputPer1M: 0.40},
}

// GetPricing returns pricing for a model using substring matching.
// Both the model name and pricing table keys are compared in lowercase.
// Returns zero pricing if no match found.
func GetPricing(model string) ModelPricing {
	modelLower := strings.ToLower(model)
	// Try exact match first
	for key, p := range pricingTable {
		if strings.ToLower(key) == modelLower {
			return p
		}
	}
	// Substring match — longest match wins
	var best ModelPricing
	bestLen := 0
	for key, p := range pricingTable {
		keyLower := strings.ToLower(key)
		if len(keyLower) > bestLen && strings.Contains(modelLower, keyLower) {
			best = p
			bestLen = len(keyLower)
		}
	}
	return best
}

// EstimateCost calculates estimated cost in USD from session usage and model name.
func EstimateCost(usage *SessionUsage, model string) float64 {
	if usage == nil {
		return 0
	}
	p := GetPricing(model)
	inputCost := float64(usage.PromptTokens) / 1_000_000 * p.InputPer1M
	outputCost := float64(usage.CompletionTokens) / 1_000_000 * p.OutputPer1M
	return inputCost + outputCost
}
