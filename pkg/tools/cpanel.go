package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// CpanelTool manages websites, domains, and databases via the cPanel UAPI.
type CpanelTool struct {
	host     string // e.g. "example.com" or "server.example.com"
	port     int    // default 2083
	username string
	apiToken string
	client   *http.Client
}

// NewCpanelTool creates a new cPanel management tool.
func NewCpanelTool(host string, port int, username, apiToken string) *CpanelTool {
	if port <= 0 {
		port = 2083
	}
	return &CpanelTool{
		host:     host,
		port:     port,
		username: username,
		apiToken: apiToken,
		client:   &http.Client{Timeout: 60 * time.Second},
	}
}

func (t *CpanelTool) Name() string {
	return "cpanel"
}

func (t *CpanelTool) Description() string {
	return "Manage a cPanel hosting account: upload/deploy website files, manage domains (addon, sub, parked, redirects), " +
		"manage MySQL databases and users, list files, and manage SSL. " +
		"Actions: file_upload, file_list, file_delete, file_create_dir, " +
		"domain_list, domain_add_addon, domain_add_sub, domain_remove, domain_redirects, " +
		"db_list, db_create, db_delete, db_create_user, db_set_privileges, db_list_users, " +
		"ssl_list, ssl_install, uapi (call any UAPI module/function directly)"
}

func (t *CpanelTool) Parameters() map[string]any {
	var schema map[string]any
	_ = json.Unmarshal([]byte(`{
		"type": "object",
		"properties": {
			"action": {
				"type": "string",
				"enum": [
					"file_upload", "file_list", "file_delete", "file_create_dir",
					"domain_list", "domain_add_addon", "domain_add_sub", "domain_remove", "domain_redirects",
					"db_list", "db_create", "db_delete", "db_create_user", "db_set_privileges", "db_list_users",
					"ssl_list", "ssl_install",
					"uapi"
				],
				"description": "Action to perform. Use 'uapi' to call any cPanel UAPI module/function directly."
			},
			"path": {
				"type": "string",
				"description": "Remote path on server (e.g. /public_html or /public_html/site). Used by file_upload, file_list, file_delete, file_create_dir."
			},
			"local_file": {
				"type": "string",
				"description": "Local file path to upload (for file_upload)"
			},
			"domain": {
				"type": "string",
				"description": "Domain name (for domain_add_addon, domain_add_sub, domain_remove, ssl_install)"
			},
			"subdomain": {
				"type": "string",
				"description": "Subdomain prefix (for domain_add_sub, e.g. 'blog')"
			},
			"document_root": {
				"type": "string",
				"description": "Document root directory (for domain_add_addon, domain_add_sub, e.g. '/blog.example.com')"
			},
			"db_name": {
				"type": "string",
				"description": "Database name (for db_create, db_delete, db_set_privileges). Will be prefixed with cpanel username automatically."
			},
			"db_user": {
				"type": "string",
				"description": "Database username (for db_create_user, db_set_privileges). Will be prefixed with cpanel username automatically."
			},
			"db_password": {
				"type": "string",
				"description": "Database user password (for db_create_user)"
			},
			"db_privileges": {
				"type": "string",
				"description": "Comma-separated privileges or 'ALL' (for db_set_privileges). Default: ALL PRIVILEGES"
			},
			"ssl_cert": {
				"type": "string",
				"description": "SSL certificate content (PEM) for ssl_install"
			},
			"ssl_key": {
				"type": "string",
				"description": "SSL private key content (PEM) for ssl_install"
			},
			"ssl_cabundle": {
				"type": "string",
				"description": "SSL CA bundle content (PEM) for ssl_install"
			},
			"redirect_url": {
				"type": "string",
				"description": "Target URL for domain redirect"
			},
			"module": {
				"type": "string",
				"description": "UAPI module name (for uapi action, e.g. 'Email', 'Ftp', 'Backup', 'CacheBuster')"
			},
			"function": {
				"type": "string",
				"description": "UAPI function name (for uapi action, e.g. 'list_pops', 'add_pop', 'fullbackup_to_homedir')"
			},
			"method": {
				"type": "string",
				"enum": ["GET", "POST"],
				"description": "HTTP method for uapi action (default: GET). Use POST for actions that modify state."
			},
			"params": {
				"type": "object",
				"description": "Key-value parameters to pass to the UAPI function (for uapi action). All values should be strings.",
				"additionalProperties": {"type": "string"}
			}
		},
		"required": ["action"]
	}`), &schema)
	return schema
}

func (t *CpanelTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	action, _ := args["action"].(string)

	switch action {
	// File management
	case "file_upload":
		return t.fileUpload(ctx, args)
	case "file_list":
		return t.fileList(ctx, args)
	case "file_delete":
		return t.fileDelete(ctx, args)
	case "file_create_dir":
		return t.fileCreateDir(ctx, args)
	// Domain management
	case "domain_list":
		return t.domainList(ctx)
	case "domain_add_addon":
		return t.domainAddAddon(ctx, args)
	case "domain_add_sub":
		return t.domainAddSub(ctx, args)
	case "domain_remove":
		return t.domainRemove(ctx, args)
	case "domain_redirects":
		return t.domainRedirects(ctx)
	// Database management
	case "db_list":
		return t.dbList(ctx)
	case "db_create":
		return t.dbCreate(ctx, args)
	case "db_delete":
		return t.dbDelete(ctx, args)
	case "db_create_user":
		return t.dbCreateUser(ctx, args)
	case "db_set_privileges":
		return t.dbSetPrivileges(ctx, args)
	case "db_list_users":
		return t.dbListUsers(ctx)
	// SSL
	case "ssl_list":
		return t.sslList(ctx)
	case "ssl_install":
		return t.sslInstall(ctx, args)
	// Generic UAPI
	case "uapi":
		return t.uapiGeneric(ctx, args)
	default:
		return ErrorResult(fmt.Sprintf("unknown action %q", action))
	}
}
