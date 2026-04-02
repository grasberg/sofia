package templates

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"text/template"
)

var (
	reFrontmatter      = regexp.MustCompile(`(?s)^---(?:\r\n|\n|\r)(.*?)(?:\r\n|\n|\r)---`)
	reStripFrontmatter = regexp.MustCompile(`(?s)^---(?:\r\n|\n|\r)(.*?)(?:\r\n|\n|\r)---(?:\r\n|\n|\r)*`)
	reYAMLList         = regexp.MustCompile(`^\[([^\]]*)\]$`)
)

// PromptTemplate is a reusable, parameterized prompt.
type PromptTemplate struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Content     string   `json:"content"`   // template text with {{.VarName}} placeholders
	Variables   []string `json:"variables"` // required variable names
	Tags        []string `json:"tags,omitempty"`
	FilePath    string   `json:"-"` // source file path
}

// TemplateManager loads and manages prompt templates.
type TemplateManager struct {
	mu        sync.RWMutex
	templates map[string]*PromptTemplate
	dirs      []string // directories to load from
}

// NewTemplateManager creates a new TemplateManager that will scan the given directories.
func NewTemplateManager(dirs ...string) *TemplateManager {
	return &TemplateManager{
		templates: make(map[string]*PromptTemplate),
		dirs:      dirs,
	}
}

// Load scans configured directories for .md template files, parses YAML frontmatter,
// and populates the template registry. Earlier directories take priority over later ones
// for templates with the same name.
func (tm *TemplateManager) Load() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.templates = make(map[string]*PromptTemplate)

	for _, dir := range tm.dirs {
		if dir == "" {
			continue
		}
		if err := tm.loadDir(dir); err != nil {
			// Skip directories that don't exist or can't be read
			continue
		}
	}
	return nil
}

// Get returns a template by name.
func (tm *TemplateManager) Get(name string) (*PromptTemplate, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	t, ok := tm.templates[name]
	return t, ok
}

// List returns all loaded templates sorted by name.
func (tm *TemplateManager) List() []*PromptTemplate {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	names := make([]string, 0, len(tm.templates))
	for name := range tm.templates {
		names = append(names, name)
	}
	sort.Strings(names)

	result := make([]*PromptTemplate, 0, len(names))
	for _, name := range names {
		result = append(result, tm.templates[name])
	}
	return result
}

// Render renders a template by name with the given variables. Returns an error if the
// template is not found or if required variables are missing.
func (tm *TemplateManager) Render(name string, vars map[string]string) (string, error) {
	tm.mu.RLock()
	pt, ok := tm.templates[name]
	tm.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("template %q not found", name)
	}

	// Check for missing required variables.
	var missing []string
	for _, v := range pt.Variables {
		if _, exists := vars[v]; !exists {
			missing = append(missing, v)
		}
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("missing required variables: %s", strings.Join(missing, ", "))
	}

	tmpl, err := template.New(name).Parse(pt.Content)
	if err != nil {
		return "", fmt.Errorf("failed to parse template %q: %w", name, err)
	}

	// Convert map[string]string to a dot-accessible map for text/template.
	data := make(map[string]string, len(vars))
	for k, v := range vars {
		data[k] = v
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render template %q: %w", name, err)
	}
	return buf.String(), nil
}

// loadDir reads all .md files from a directory and parses them as templates.
// Templates already loaded (from higher-priority dirs) are not overwritten.
func (tm *TemplateManager) loadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		filePath := filepath.Join(dir, entry.Name())
		pt, err := tm.parseTemplateFile(filePath)
		if err != nil {
			continue // skip invalid files
		}
		// First directory wins — don't overwrite existing templates.
		if _, exists := tm.templates[pt.Name]; !exists {
			tm.templates[pt.Name] = pt
		}
	}
	return nil
}

// parseTemplateFile reads a Markdown file with YAML frontmatter and returns a PromptTemplate.
func (tm *TemplateManager) parseTemplateFile(filePath string) (*PromptTemplate, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	raw := string(data)
	frontmatter := extractFrontmatter(raw)
	if frontmatter == "" {
		return nil, fmt.Errorf("no frontmatter in %s", filePath)
	}

	meta := parseSimpleYAML(frontmatter)
	name := meta["name"]
	if name == "" {
		// Fall back to the filename without extension.
		name = strings.TrimSuffix(filepath.Base(filePath), ".md")
	}

	content := stripFrontmatter(raw)

	pt := &PromptTemplate{
		Name:        name,
		Description: meta["description"],
		Content:     content,
		Variables:   parseYAMLList(meta["variables"]),
		Tags:        parseYAMLList(meta["tags"]),
		FilePath:    filePath,
	}
	return pt, nil
}

// extractFrontmatter pulls the YAML block between --- delimiters.
func extractFrontmatter(content string) string {
	match := reFrontmatter.FindStringSubmatch(content)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

// stripFrontmatter removes the YAML frontmatter block from the content.
func stripFrontmatter(content string) string {
	return reStripFrontmatter.ReplaceAllString(content, "")
}

// parseSimpleYAML parses simple key: value YAML lines. Handles \r\n, \r, and \n line endings.
func parseSimpleYAML(content string) map[string]string {
	result := make(map[string]string)

	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")

	for _, line := range strings.Split(normalized, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			value = strings.Trim(value, "\"'")
			result[key] = value
		}
	}
	return result
}

// parseYAMLList parses a YAML inline list like "[a, b, c]" into a string slice.
// Returns nil for empty input.
func parseYAMLList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	match := reYAMLList.FindStringSubmatch(raw)
	if len(match) < 2 {
		// Not bracket-wrapped — treat as a single item.
		return []string{strings.TrimSpace(raw)}
	}

	inner := match[1]
	if strings.TrimSpace(inner) == "" {
		return nil
	}

	parts := strings.Split(inner, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
