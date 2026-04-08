package recipe

// Recipe defines a repeatable, parameterised agent workflow that can be loaded
// from YAML, discovered from well-known directories, and executed by the agent
// loop. Recipes add instructions to the system prompt, render a templated user
// prompt, optionally enforce structured output, and support retry-with-checks.
type Recipe struct {
	Version      string          `yaml:"version"`
	Title        string          `yaml:"title"`
	Description  string          `yaml:"description"`
	Instructions string          `yaml:"instructions"` // added to system prompt
	Prompt       string          `yaml:"prompt"`       // initial user message (supports {{param}} templates)
	Extensions   []ExtensionRef  `yaml:"extensions,omitempty"`
	Settings     RecipeSettings  `yaml:"settings,omitempty"`
	Parameters   []RecipeParam   `yaml:"parameters,omitempty"`
	Response     *ResponseSchema `yaml:"response,omitempty"` // structured output schema
	Retry        *RetryConfig    `yaml:"retry,omitempty"`
	SubRecipes   []SubRecipeRef  `yaml:"sub_recipes,omitempty"`
	Author       string          `yaml:"author,omitempty"`
}

// RecipeSettings overrides agent-level model / provider defaults for the
// duration of a recipe execution.
type RecipeSettings struct {
	Provider    string  `yaml:"provider,omitempty"`
	Model       string  `yaml:"model,omitempty"`
	Temperature float64 `yaml:"temperature,omitempty"`
	MaxTurns    int     `yaml:"max_turns,omitempty"`
}

// RecipeParam describes a single user-supplied parameter.
// InputType is one of: string, number, boolean, file, select.
// Required uses the YAML tag "requirement" so authors write requirement: required.
type RecipeParam struct {
	Key         string   `yaml:"key"`
	Description string   `yaml:"description,omitempty"`
	InputType   string   `yaml:"input_type"` // string, number, boolean, file, select
	Required    bool     `yaml:"requirement"`
	Default     string   `yaml:"default,omitempty"`
	Options     []string `yaml:"options,omitempty"` // for select type
}

// ResponseSchema optionally constrains the final LLM output to a JSON schema.
type ResponseSchema struct {
	JSONSchema map[string]any `yaml:"json_schema,omitempty"`
}

// RetryConfig controls automatic retry behaviour after the LLM loop completes.
type RetryConfig struct {
	MaxRetries int          `yaml:"max_retries"`
	Checks     []RetryCheck `yaml:"checks,omitempty"`
	OnFailure  string       `yaml:"on_failure,omitempty"` // shell command to run on failure
}

// RetryCheck is a single validation step executed after the LLM loop.
type RetryCheck struct {
	Shell ShellCheck `yaml:"shell"`
}

// ShellCheck runs a shell command; exit 0 means success.
type ShellCheck struct {
	Command string `yaml:"command"`
}

// ExtensionRef references an MCP server or tool extension by name.
type ExtensionRef struct {
	Name string `yaml:"name"`
}

// SubRecipeRef references a child recipe that can be invoked during execution.
type SubRecipeRef struct {
	Name string `yaml:"name"`
	Path string `yaml:"path"`
}

// RecipeMeta is the lightweight descriptor returned by ListRecipes.
type RecipeMeta struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Source      string `json:"source"` // "workspace" or "global"
}
