package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var templateNamePattern = regexp.MustCompile(`^[a-zA-Z0-9]+(?:-[a-zA-Z0-9]+)*$`)

type PurposeTemplate struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Skills       []string `json:"skills,omitempty"`
	Instructions string   `json:"instructions,omitempty"`
	Path         string   `json:"path,omitempty"`
}

func ResolveAntigravityKitDir() string {
	if custom := os.Getenv("SOFIA_ANTIGRAVITY_DIR"); custom != "" {
		if st, err := os.Stat(custom); err == nil && st.IsDir() {
			return custom
		}
	}

	home, _ := os.UserHomeDir()
	wd, _ := os.Getwd()
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)

	candidates := []string{
		filepath.Join(home, ".sofia", "antigravity-kit"),
		filepath.Join(wd, "third_party", "antigravity-kit"),
		filepath.Join(exeDir, "..", "share", "sofia", "antigravity-kit"),
	}

	for _, dir := range candidates {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			return dir
		}
	}

	return filepath.Join(home, ".sofia", "antigravity-kit")
}

func ListPurposeTemplates() ([]PurposeTemplate, error) {
	templatesDir := filepath.Join(ResolveAntigravityKitDir(), ".agent", "agents")
	entries, err := os.ReadDir(templatesDir)
	if err != nil {
		return nil, err
	}

	out := make([]PurposeTemplate, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".md" {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		t, err := LoadPurposeTemplate(name)
		if err != nil {
			continue
		}
		out = append(out, *t)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func LoadPurposeTemplate(name string) (*PurposeTemplate, error) {
	if !templateNamePattern.MatchString(name) {
		return nil, fmt.Errorf("invalid template name: %q", name)
	}

	filePath := filepath.Join(ResolveAntigravityKitDir(), ".agent", "agents", name+".md")
	b, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	parsed := parseTemplateMarkdown(string(b))
	parsed.Name = name
	parsed.Path = filePath
	return &parsed, nil
}

func parseTemplateMarkdown(content string) PurposeTemplate {
	t := PurposeTemplate{}
	body := content

	if strings.HasPrefix(content, "---\n") {
		parts := strings.SplitN(content, "\n---\n", 2)
		if len(parts) == 2 {
			frontmatter := strings.TrimPrefix(parts[0], "---\n")
			body = parts[1]
			meta := parseSimpleYAML(frontmatter)
			t.Description = meta["description"]
			t.Skills = parseSkills(meta["skills"])
		}
	}

	t.Instructions = strings.TrimSpace(body)
	return t
}

func parseSimpleYAML(content string) map[string]string {
	result := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		result[key] = strings.Trim(val, "\"'")
	}
	return result
}

func parseSkills(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
		raw = strings.TrimPrefix(raw, "[")
		raw = strings.TrimSuffix(raw, "]")
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.Trim(strings.TrimSpace(p), "\"'")
		if v != "" {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
