package tools

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// ── Database management ──────────────────────────────────────────────

func (t *CpanelTool) dbList(ctx context.Context) *ToolResult {
	result, err := t.doGet(ctx, "Mysql", "list_databases", nil)
	if err != nil {
		return RetryableError(fmt.Sprintf("list databases failed: %v", err), "Check cPanel connection")
	}

	data, err := uapiOK(result)
	if err != nil {
		return ErrorResult(fmt.Sprintf("list databases: %v", err))
	}

	dbs, _ := data.([]any)
	if len(dbs) == 0 {
		return NewToolResult("No MySQL databases found.")
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "**MySQL Databases (%d):**\n\n", len(dbs))
	sb.WriteString("| Database | Size | Users |\n")
	sb.WriteString("|----------|------|-------|\n")

	for _, d := range dbs {
		dm, ok := d.(map[string]any)
		if !ok {
			continue
		}
		name, _ := dm["database"].(string)
		size, _ := dm["disk_usage"].(float64)

		users := ""
		if uList, ok := dm["users"].([]any); ok {
			var userNames []string
			for _, u := range uList {
				if s, ok := u.(string); ok {
					userNames = append(userNames, s)
				}
			}
			users = strings.Join(userNames, ", ")
		}

		sizeStr := fmt.Sprintf("%.1f MB", size/1024/1024)
		if size < 1024*1024 {
			sizeStr = fmt.Sprintf("%.1f KB", size/1024)
		}
		fmt.Fprintf(&sb, "| %s | %s | %s |\n", name, sizeStr, users)
	}

	return NewToolResult(sb.String())
}

func (t *CpanelTool) dbCreate(ctx context.Context, args map[string]any) *ToolResult {
	name := getStr(args, "db_name")
	if name == "" {
		return ErrorResult("db_name is required for db_create")
	}

	params := url.Values{}
	params.Set("name", name)

	result, err := t.doPost(ctx, "Mysql", "create_database", params)
	if err != nil {
		return ErrorResult(fmt.Sprintf("create database failed: %v", err))
	}

	if _, err := uapiOK(result); err != nil {
		return ErrorResult(fmt.Sprintf("create database: %v", err))
	}

	return NewToolResult(fmt.Sprintf("**Database created:** `%s`", name))
}

func (t *CpanelTool) dbDelete(ctx context.Context, args map[string]any) *ToolResult {
	name := getStr(args, "db_name")
	if name == "" {
		return ErrorResult("db_name is required for db_delete")
	}

	params := url.Values{}
	params.Set("name", name)

	result, err := t.doPost(ctx, "Mysql", "delete_database", params)
	if err != nil {
		return ErrorResult(fmt.Sprintf("delete database failed: %v", err))
	}

	if _, err := uapiOK(result); err != nil {
		return ErrorResult(fmt.Sprintf("delete database: %v", err))
	}

	return NewToolResult(fmt.Sprintf("**Database deleted:** `%s`", name))
}

func (t *CpanelTool) dbCreateUser(ctx context.Context, args map[string]any) *ToolResult {
	user := getStr(args, "db_user")
	password := getStr(args, "db_password")
	if user == "" {
		return ErrorResult("db_user is required for db_create_user")
	}
	if password == "" {
		return ErrorResult("db_password is required for db_create_user")
	}

	params := url.Values{}
	params.Set("name", user)
	params.Set("password", password)

	result, err := t.doPost(ctx, "Mysql", "create_user", params)
	if err != nil {
		return ErrorResult(fmt.Sprintf("create user failed: %v", err))
	}

	if _, err := uapiOK(result); err != nil {
		return ErrorResult(fmt.Sprintf("create user: %v", err))
	}

	return NewToolResult(fmt.Sprintf("**Database user created:** `%s`", user))
}

func (t *CpanelTool) dbSetPrivileges(ctx context.Context, args map[string]any) *ToolResult {
	dbName := getStr(args, "db_name")
	dbUser := getStr(args, "db_user")
	if dbName == "" {
		return ErrorResult("db_name is required for db_set_privileges")
	}
	if dbUser == "" {
		return ErrorResult("db_user is required for db_set_privileges")
	}

	privileges := getStr(args, "db_privileges")
	if privileges == "" {
		privileges = "ALL PRIVILEGES"
	}

	params := url.Values{}
	params.Set("user", dbUser)
	params.Set("database", dbName)
	params.Set("privileges", privileges)

	result, err := t.doPost(ctx, "Mysql", "set_privileges_on_database", params)
	if err != nil {
		return ErrorResult(fmt.Sprintf("set privileges failed: %v", err))
	}

	if _, err := uapiOK(result); err != nil {
		return ErrorResult(fmt.Sprintf("set privileges: %v", err))
	}

	return NewToolResult(fmt.Sprintf("**Privileges set:** `%s` on `%s` → %s", dbUser, dbName, privileges))
}

func (t *CpanelTool) dbListUsers(ctx context.Context) *ToolResult {
	result, err := t.doGet(ctx, "Mysql", "list_users", nil)
	if err != nil {
		return RetryableError(fmt.Sprintf("list users failed: %v", err), "Check cPanel connection")
	}

	data, err := uapiOK(result)
	if err != nil {
		return ErrorResult(fmt.Sprintf("list users: %v", err))
	}

	users, _ := data.([]any)
	if len(users) == 0 {
		return NewToolResult("No MySQL users found.")
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "**MySQL Users (%d):**\n", len(users))
	for i, u := range users {
		if s, ok := u.(string); ok {
			fmt.Fprintf(&sb, "%d. `%s`\n", i+1, s)
		} else if m, ok := u.(map[string]any); ok {
			name, _ := m["user"].(string)
			fmt.Fprintf(&sb, "%d. `%s`\n", i+1, name)
		}
	}

	return NewToolResult(sb.String())
}
