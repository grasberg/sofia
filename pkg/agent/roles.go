package agent

import "sort"

// RoleTemplate defines a pre-built agent archetype.
type RoleTemplate struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	SystemPrompt   string   `json:"system_prompt"`
	SuggestedTools []string `json:"suggested_tools,omitempty"`
	Temperature    float64  `json:"temperature,omitempty"`
}

// BuiltinRoles contains the default role templates shipped with Sofia.
var BuiltinRoles = map[string]RoleTemplate{
	"researcher": {
		Name:        "Researcher",
		Description: "Deep research and analysis with web search and document review",
		SystemPrompt: "You are a thorough research assistant. Always cite sources, " +
			"cross-reference claims, and present balanced analysis. Prefer depth over breadth. " +
			"Structure findings with clear sections and evidence.",
		SuggestedTools: []string{"web_search", "web_fetch", "read_file", "write_file"},
		Temperature:    0.3,
	},
	"writer": {
		Name:        "Writer",
		Description: "Creative and technical writing with editing capabilities",
		SystemPrompt: "You are an expert writer and editor. Adapt your tone and style to the " +
			"audience. Focus on clarity, conciseness, and engaging prose. Offer structural " +
			"suggestions and alternative phrasings.",
		SuggestedTools: []string{"read_file", "write_file", "edit_file"},
		Temperature:    0.7,
	},
	"developer": {
		Name:        "Developer",
		Description: "Software development with code review and testing",
		SystemPrompt: "You are a senior software engineer. Write clean, efficient, well-tested " +
			"code. Follow established patterns in the codebase. Explain technical decisions. " +
			"Prefer simple solutions over clever ones.",
		SuggestedTools: []string{"read_file", "write_file", "edit_file", "exec", "list_dir"},
		Temperature:    0.2,
	},
	"devops": {
		Name:        "DevOps",
		Description: "Infrastructure, deployment, and system administration",
		SystemPrompt: "You are a DevOps engineer. Focus on reliability, automation, and security. " +
			"Prefer infrastructure-as-code. Explain the 'why' behind configuration decisions. " +
			"Always consider failure modes.",
		SuggestedTools: []string{"exec", "read_file", "write_file", "list_dir"},
		Temperature:    0.2,
	},
	"analyst": {
		Name:        "Analyst",
		Description: "Data analysis, reporting, and business intelligence",
		SystemPrompt: "You are a data analyst. Present findings clearly with supporting evidence. " +
			"Use structured formats for data. Identify trends and anomalies. Qualify conclusions " +
			"with confidence levels.",
		SuggestedTools: []string{"exec", "read_file", "write_file", "web_search"},
		Temperature:    0.3,
	},
	"assistant": {
		Name:        "Personal Assistant",
		Description: "General-purpose personal assistant for tasks and organization",
		SystemPrompt: "You are a capable personal assistant. Be proactive, organized, and " +
			"thorough. Anticipate needs. Keep track of details. Communicate clearly and concisely.",
		SuggestedTools: []string{"web_search", "web_fetch", "exec", "read_file", "write_file"},
		Temperature:    0.5,
	},
}

// GetBuiltinRole returns the RoleTemplate for the given name. The second return
// value is false when no matching role exists.
func GetBuiltinRole(name string) (RoleTemplate, bool) {
	r, ok := BuiltinRoles[name]
	return r, ok
}

// ListBuiltinRoles returns all built-in role templates sorted by name.
func ListBuiltinRoles() []RoleTemplate {
	keys := make([]string, 0, len(BuiltinRoles))
	for k := range BuiltinRoles {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	roles := make([]RoleTemplate, 0, len(keys))
	for _, k := range keys {
		roles = append(roles, BuiltinRoles[k])
	}
	return roles
}
