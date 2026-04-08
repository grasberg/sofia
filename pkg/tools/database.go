package tools

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// DatabaseTool provides SQL query execution against SQLite, PostgreSQL, and MySQL.
type DatabaseTool struct{}

func NewDatabaseTool() *DatabaseTool { return &DatabaseTool{} }

func (t *DatabaseTool) Name() string { return "database" }
func (t *DatabaseTool) Description() string {
	return "Execute SQL queries against databases. Supports SQLite (built-in), PostgreSQL, and MySQL via connection strings. Actions: query (read data), exec (write data), schema (inspect tables), tables (list tables)."
}

func (t *DatabaseTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"query", "exec", "schema", "tables"},
				"description": "Action: query (SELECT), exec (INSERT/UPDATE/DELETE), schema (table DDL), tables (list all tables)",
			},
			"driver": map[string]any{
				"type":        "string",
				"enum":        []string{"sqlite", "postgres", "mysql"},
				"description": "Database driver (default: sqlite)",
			},
			"dsn": map[string]any{
				"type":        "string",
				"description": "Data source name / connection string. For sqlite: file path. For postgres: postgres://user:pass@host/db. For mysql: user:pass@tcp(host)/db.",
			},
			"sql": map[string]any{
				"type":        "string",
				"description": "SQL query to execute",
			},
			"table": map[string]any{
				"type":        "string",
				"description": "Table name (for schema action)",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Max rows to return (default 100)",
			},
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"description": "Query timeout in seconds (default 30)",
			},
		},
		"required": []string{"action", "dsn"},
	}
}

func (t *DatabaseTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	action, _ := args["action"].(string)
	dsn, _ := args["dsn"].(string)

	if action == "" || dsn == "" {
		return ErrorResult("action and dsn are required")
	}

	driver := "sqlite"
	if d, ok := args["driver"].(string); ok && d != "" {
		driver = d
	}

	timeout := 30 * time.Second
	if raw, ok := args["timeout_seconds"]; ok {
		if n, ok := parsePositiveInt(raw); ok {
			timeout = time.Duration(n) * time.Second
		}
	}

	// Map driver names to sql.Open driver names
	sqlDriver := driver
	switch driver {
	case "sqlite":
		sqlDriver = "sqlite"
	case "postgres":
		sqlDriver = "pgx"
	case "mysql":
		sqlDriver = "mysql"
	}

	db, err := sql.Open(sqlDriver, dsn)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to open database: %v", err))
	}
	defer db.Close()

	queryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := db.PingContext(queryCtx); err != nil {
		return ErrorResult(
			fmt.Sprintf("failed to connect: %v (driver: %s, ensure the driver is available)", err, driver),
		)
	}

	switch action {
	case "query":
		return t.executeQuery(queryCtx, db, args)
	case "exec":
		return t.executeExec(queryCtx, db, args)
	case "schema":
		return t.getSchema(queryCtx, db, driver, args)
	case "tables":
		return t.listTables(queryCtx, db, driver)
	default:
		return ErrorResult(fmt.Sprintf("unknown action: %s", action))
	}
}

func (t *DatabaseTool) executeQuery(ctx context.Context, db *sql.DB, args map[string]any) *ToolResult {
	query, ok := args["sql"].(string)
	if !ok || query == "" {
		return ErrorResult("sql is required for query action")
	}

	// Safety: block write operations in query mode
	upper := strings.ToUpper(strings.TrimSpace(query))
	if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") &&
		!strings.HasPrefix(upper, "EXPLAIN") && !strings.HasPrefix(upper, "PRAGMA") {
		return ErrorResult("query action only supports SELECT/WITH/EXPLAIN statements. Use exec for writes.")
	}

	limit := 100
	if raw, ok := args["limit"]; ok {
		if n, ok := parsePositiveInt(raw); ok {
			limit = n
		}
	}

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return ErrorResult(fmt.Sprintf("query failed: %v", err))
	}
	defer rows.Close()

	return formatRows(rows, limit)
}

func (t *DatabaseTool) executeExec(ctx context.Context, db *sql.DB, args map[string]any) *ToolResult {
	query, ok := args["sql"].(string)
	if !ok || query == "" {
		return ErrorResult("sql is required for exec action")
	}

	result, err := db.ExecContext(ctx, query)
	if err != nil {
		return ErrorResult(fmt.Sprintf("exec failed: %v", err))
	}

	rowsAffected, _ := result.RowsAffected()
	lastID, _ := result.LastInsertId()

	msg := fmt.Sprintf("Rows affected: %d", rowsAffected)
	if lastID > 0 {
		msg += fmt.Sprintf(", Last insert ID: %d", lastID)
	}
	return SilentResult(msg)
}

func (t *DatabaseTool) getSchema(ctx context.Context, db *sql.DB, driver string, args map[string]any) *ToolResult {
	table, ok := args["table"].(string)
	if !ok || table == "" {
		return ErrorResult("table is required for schema action")
	}

	var query string
	switch driver {
	case "sqlite":
		query = fmt.Sprintf("PRAGMA table_info(%s)", table)
	case "postgres":
		query = fmt.Sprintf(`SELECT column_name, data_type, is_nullable, column_default
			FROM information_schema.columns WHERE table_name = '%s' ORDER BY ordinal_position`, table)
	case "mysql":
		query = fmt.Sprintf("DESCRIBE %s", table)
	}

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return ErrorResult(fmt.Sprintf("schema query failed: %v", err))
	}
	defer rows.Close()

	return formatRows(rows, 200)
}

func (t *DatabaseTool) listTables(ctx context.Context, db *sql.DB, driver string) *ToolResult {
	var query string
	switch driver {
	case "sqlite":
		query = "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name"
	case "postgres":
		query = "SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename"
	case "mysql":
		query = "SHOW TABLES"
	}

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return ErrorResult(fmt.Sprintf("list tables failed: %v", err))
	}
	defer rows.Close()

	return formatRows(rows, 500)
}

func formatRows(rows *sql.Rows, limit int) *ToolResult {
	cols, err := rows.Columns()
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to get columns: %v", err))
	}

	var sb strings.Builder

	// Header
	sb.WriteString(strings.Join(cols, " | "))
	sb.WriteByte('\n')
	for range cols {
		sb.WriteString("---")
		sb.WriteString(" | ")
	}
	sb.WriteByte('\n')

	// Rows
	count := 0
	values := make([]any, len(cols))
	valuePtrs := make([]any, len(cols))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	for rows.Next() {
		if count >= limit {
			sb.WriteString(fmt.Sprintf("\n... (limited to %d rows)", limit))
			break
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}
		for i, v := range values {
			if i > 0 {
				sb.WriteString(" | ")
			}
			sb.WriteString(fmt.Sprintf("%v", v))
		}
		sb.WriteByte('\n')
		count++
	}

	if count == 0 {
		return NewToolResult("(no rows)")
	}

	sb.WriteString(fmt.Sprintf("\n(%d rows)", count))
	return NewToolResult(sb.String())
}
