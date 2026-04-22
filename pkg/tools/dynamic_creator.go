package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/memory"
)

// DynamicToolCreator is the meta-tool that creates, lists, and removes
// dynamic tools at runtime.
type DynamicToolCreator struct {
	db        *memory.MemoryDB
	registry  *ToolRegistry
	workspace string
}

// NewDynamicToolCreator creates the dynamic tool management tool.
func NewDynamicToolCreator(
	db *memory.MemoryDB,
	registry *ToolRegistry,
	workspace string,
) *DynamicToolCreator {
	return &DynamicToolCreator{
		db:        db,
		registry:  registry,
		workspace: workspace,
	}
}

func (t *DynamicToolCreator) Name() string {
	return "dynamic_tool"
}

func (t *DynamicToolCreator) Description() string {
	return "Create, list, or remove tools at runtime. " +
		"Created tools become immediately available. " +
		"Tools persist across sessions. " +
		"Use 'command' for shell-based tools or " +
		"'template' for Go text/template-based tools."
}

func (t *DynamicToolCreator) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{
				"type": "string",
				"enum": []string{
					"create", "list", "remove", "get",
				},
				"description": "The operation to perform",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Tool name (lowercase, underscores)",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "What the tool does (for create)",
			},
			"parameters": map[string]any{
				"type": "object",
				"description": "JSON Schema for the tool's " +
					"parameters (for create)",
			},
			"command": map[string]any{
				"type": "string",
				"description": "Shell command to run, use " +
					"{{.argName}} for arg interpolation",
			},
			"template": map[string]any{
				"type": "string",
				"description": "Go text/template producing " +
					"the result, use {{.argName}} for args",
			},
		},
		"required": []string{"operation"},
	}
}

func (t *DynamicToolCreator) Execute(
	_ context.Context, args map[string]any,
) *ToolResult {
	op, _ := args["operation"].(string) //nolint:errcheck

	switch op {
	case "create":
		return t.create(args)
	case "list":
		return t.list()
	case "remove":
		return t.remove(args)
	case "get":
		return t.get(args)
	default:
		return ErrorResult(fmt.Sprintf(
			"unknown operation %q: use create, list, "+
				"remove, or get", op,
		))
	}
}

func (t *DynamicToolCreator) create(
	args map[string]any,
) *ToolResult {
	name, _ := args["name"].(string) //nolint:errcheck
	if name == "" {
		return ErrorResult("name is required")
	}

	for _, c := range name {
		valid := (c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') || c == '_'
		if !valid {
			return ErrorResult(
				"name must contain only lowercase letters, " +
					"digits, and underscores",
			)
		}
	}

	if existing, ok := t.registry.Get(name); ok {
		if _, isDynamic := existing.(*DynamicTool); !isDynamic {
			return ErrorResult(fmt.Sprintf(
				"cannot overwrite built-in tool %q", name,
			))
		}
	}

	desc, _ := args["description"].(string) //nolint:errcheck
	if desc == "" {
		return ErrorResult("description is required")
	}

	command, _ := args["command"].(string) //nolint:errcheck
	tmpl, _ := args["template"].(string)   //nolint:errcheck
	if command == "" && tmpl == "" {
		return ErrorResult(
			"either command or template is required",
		)
	}
	if command != "" && tmpl != "" {
		return ErrorResult(
			"command and template are mutually exclusive",
		)
	}

	if tmpl != "" {
		if _, err := template.New("v").Parse(tmpl); err != nil {
			return ErrorResult(fmt.Sprintf(
				"invalid template: %v", err,
			))
		}
	}
	if command != "" {
		if _, err := template.New("v").Parse(command); err != nil {
			return ErrorResult(fmt.Sprintf(
				"invalid command template: %v", err,
			))
		}
	}

	params := map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
	if p, ok := args["parameters"].(map[string]any); ok {
		params = p
	}

	def := DynamicToolDef{
		Name:        name,
		Description: desc,
		Parameters:  params,
		Command:     command,
		Template:    tmpl,
	}

	defJSON, err := json.Marshal(def)
	if err != nil {
		return ErrorResult(
			fmt.Sprintf("marshal error: %v", err),
		)
	}

	_, err = t.db.Exec(
		`INSERT OR REPLACE INTO dynamic_tools
		 (name, definition) VALUES (?, ?)`,
		name, string(defJSON),
	)
	if err != nil {
		return ErrorResult(
			fmt.Sprintf("save error: %v", err),
		)
	}

	tool := NewDynamicTool(def, t.workspace)
	t.registry.Register(tool)

	logger.InfoCF("dynamic-tool", "Created dynamic tool",
		map[string]any{"name": name})

	return NewToolResult(fmt.Sprintf(
		"Created tool %q — it is now available for use.\n"+
			"Description: %s", name, desc,
	))
}

func (t *DynamicToolCreator) list() *ToolResult {
	rows, err := t.db.Query(
		`SELECT name, definition FROM dynamic_tools
		 ORDER BY name`,
	)
	if err != nil {
		return ErrorResult(
			fmt.Sprintf("query error: %v", err),
		)
	}
	defer func() { _ = rows.Close() }() //nolint:errcheck

	var sb strings.Builder
	count := 0
	for rows.Next() {
		var name, defJSON string
		if err := rows.Scan(&name, &defJSON); err != nil {
			continue
		}
		var def DynamicToolDef
		if err := json.Unmarshal(
			[]byte(defJSON), &def,
		); err != nil {
			continue
		}
		count++
		kind := "command"
		if def.Template != "" {
			kind = "template"
		}
		fmt.Fprintf(&sb, "  - %s [%s]: %s\n",
			def.Name, kind, def.Description)
	}

	if count == 0 {
		return NewToolResult("No dynamic tools defined.")
	}
	return NewToolResult(fmt.Sprintf(
		"%d dynamic tool(s):\n%s", count, sb.String(),
	))
}

func (t *DynamicToolCreator) remove(
	args map[string]any,
) *ToolResult {
	name, _ := args["name"].(string) //nolint:errcheck
	if name == "" {
		return ErrorResult("name is required")
	}

	if existing, ok := t.registry.Get(name); ok {
		if _, isDynamic := existing.(*DynamicTool); !isDynamic {
			return ErrorResult(fmt.Sprintf(
				"cannot remove built-in tool %q", name,
			))
		}
	}

	_, err := t.db.Exec(
		`DELETE FROM dynamic_tools WHERE name = ?`, name,
	)
	if err != nil {
		return ErrorResult(
			fmt.Sprintf("delete error: %v", err),
		)
	}

	t.registry.Unregister(name)

	logger.InfoCF("dynamic-tool", "Removed dynamic tool",
		map[string]any{"name": name})

	return NewToolResult(
		fmt.Sprintf("Removed tool %q", name),
	)
}

func (t *DynamicToolCreator) get(
	args map[string]any,
) *ToolResult {
	name, _ := args["name"].(string) //nolint:errcheck
	if name == "" {
		return ErrorResult("name is required")
	}

	row := t.db.QueryRow(
		`SELECT definition FROM dynamic_tools WHERE name = ?`,
		name,
	)
	var defJSON string
	if err := row.Scan(&defJSON); err != nil {
		return ErrorResult(
			fmt.Sprintf("tool %q not found", name),
		)
	}

	var def DynamicToolDef
	if err := json.Unmarshal(
		[]byte(defJSON), &def,
	); err != nil {
		return ErrorResult(
			fmt.Sprintf("parse error: %v", err),
		)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Tool: %s\n", def.Name)
	fmt.Fprintf(&sb, "Description: %s\n", def.Description)
	if def.Command != "" {
		fmt.Fprintf(&sb, "Type: command\nCommand: %s\n",
			def.Command)
	}
	if def.Template != "" {
		fmt.Fprintf(&sb, "Type: template\nTemplate: %s\n",
			def.Template)
	}
	paramsJSON, _ := json.MarshalIndent( //nolint:errcheck
		def.Parameters, "", "  ",
	)
	fmt.Fprintf(&sb, "Parameters: %s\n", string(paramsJSON))
	return NewToolResult(sb.String())
}

// LoadDynamicTools loads all persisted dynamic tools and registers
// them into the given registry.
func LoadDynamicTools(
	db *memory.MemoryDB,
	registry *ToolRegistry,
	workspace string,
) {
	rows, err := db.Query(
		`SELECT name, definition FROM dynamic_tools
		 ORDER BY name`,
	)
	if err != nil {
		logger.WarnCF("dynamic-tool",
			"Failed to load dynamic tools",
			map[string]any{"error": err.Error()})
		return
	}
	defer func() { _ = rows.Close() }() //nolint:errcheck

	count := 0
	for rows.Next() {
		var name, defJSON string
		if err := rows.Scan(&name, &defJSON); err != nil {
			continue
		}
		var def DynamicToolDef
		if err := json.Unmarshal(
			[]byte(defJSON), &def,
		); err != nil {
			logger.WarnCF("dynamic-tool",
				"Failed to parse dynamic tool",
				map[string]any{
					"name":  name,
					"error": err.Error(),
				})
			continue
		}
		registry.Register(NewDynamicTool(def, workspace))
		count++
	}

	if count > 0 {
		logger.InfoCF("dynamic-tool",
			fmt.Sprintf("Loaded %d dynamic tool(s)", count),
			nil)
	}
}
