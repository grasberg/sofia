package agent

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
}

// GetPricing returns pricing for a model using substring matching.
// Returns zero pricing if no match found.
func GetPricing(model string) ModelPricing {
	// Try exact match first
	if p, ok := pricingTable[model]; ok {
		return p
	}
	// Substring match — longest match wins
	var best ModelPricing
	bestLen := 0
	for key, p := range pricingTable {
		if len(key) > bestLen && containsSubstring(model, key) {
			best = p
			bestLen = len(key)
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

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
