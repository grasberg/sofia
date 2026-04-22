package tools

import (
	"github.com/grasberg/sofia/pkg/logger"
)

// InspectionVerdict represents the outcome of inspecting a tool call.
type InspectionVerdict struct {
	Allowed    bool
	Reason     string
	Inspector  string  // which inspector flagged it
	RiskLevel  string  // "low", "medium", "high", "critical"
	Confidence float64 // 0.0 to 1.0
}

// allowedVerdict is a convenience constructor for a passing verdict.
func allowedVerdict(inspector string) *InspectionVerdict {
	return &InspectionVerdict{
		Allowed:    true,
		Inspector:  inspector,
		RiskLevel:  "low",
		Confidence: 1.0,
	}
}

// blockedVerdict is a convenience constructor for a blocking verdict.
func blockedVerdict(inspector, reason, riskLevel string, confidence float64) *InspectionVerdict {
	return &InspectionVerdict{
		Allowed:    false,
		Reason:     reason,
		Inspector:  inspector,
		RiskLevel:  riskLevel,
		Confidence: confidence,
	}
}

// ToolInspector inspects a tool call before execution.
type ToolInspector interface {
	Name() string
	Inspect(toolName string, args map[string]any, argsJSON string) *InspectionVerdict
}

// InspectionPipeline chains multiple inspectors, executing them in order.
// The first non-allowed verdict short-circuits the pipeline.
type InspectionPipeline struct {
	inspectors []ToolInspector
}

// NewInspectionPipeline constructs a pipeline from the given inspectors.
func NewInspectionPipeline(inspectors ...ToolInspector) *InspectionPipeline {
	return &InspectionPipeline{inspectors: inspectors}
}

// Inspect runs every inspector in registration order. If any inspector returns
// a non-allowed verdict, the pipeline short-circuits and returns that verdict.
// When all inspectors pass, an allowed verdict is returned.
func (p *InspectionPipeline) Inspect(toolName string, args map[string]any, argsJSON string) *InspectionVerdict {
	for _, insp := range p.inspectors {
		v := insp.Inspect(toolName, args, argsJSON)
		if v != nil && !v.Allowed {
			logger.InfoCF("inspection", "Tool call blocked",
				map[string]any{
					"tool":      toolName,
					"inspector": v.Inspector,
					"reason":    v.Reason,
					"risk":      v.RiskLevel,
				})
			return v
		}
	}
	return allowedVerdict("pipeline")
}

// AddInspector appends an inspector to the pipeline.
func (p *InspectionPipeline) AddInspector(insp ToolInspector) {
	p.inspectors = append(p.inspectors, insp)
}
