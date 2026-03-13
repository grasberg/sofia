package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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

// ── UAPI helpers ─────────────────────────────────────────────────────

func (t *CpanelTool) baseURL() string {
	return fmt.Sprintf("https://%s:%d", t.host, t.port)
}

func (t *CpanelTool) uapiURL(module, function string) string {
	return fmt.Sprintf("%s/execute/%s/%s", t.baseURL(), module, function)
}

func (t *CpanelTool) doGet(ctx context.Context, module, function string, params url.Values) (map[string]any, error) {
	reqURL := t.uapiURL(module, function)
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("cpanel %s:%s", t.username, t.apiToken))

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateStr(string(body), 300))
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return result, nil
}

func (t *CpanelTool) doPost(ctx context.Context, module, function string, params url.Values) (map[string]any, error) {
	reqURL := t.uapiURL(module, function)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("cpanel %s:%s", t.username, t.apiToken))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateStr(string(body), 300))
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return result, nil
}

func uapiOK(result map[string]any) (any, error) {
	status, _ := result["status"].(float64)
	if status != 1 {
		errs, _ := result["errors"].([]any)
		if len(errs) > 0 {
			var msgs []string
			for _, e := range errs {
				if s, ok := e.(string); ok {
					msgs = append(msgs, s)
				}
			}
			return nil, fmt.Errorf("%s", strings.Join(msgs, "; "))
		}
		return nil, fmt.Errorf("cPanel API error (status %v)", result["status"])
	}
	return result["data"], nil
}

func getStr(args map[string]any, key string) string {
	s, _ := args[key].(string)
	return strings.TrimSpace(s)
}

func validateRemotePath(path string) error {
	if strings.Contains(path, "..") {
		return fmt.Errorf("path must not contain '..'")
	}
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("path must be absolute (start with /)")
	}
	return nil
}

// ── File management ──────────────────────────────────────────────────

func (t *CpanelTool) fileUpload(ctx context.Context, args map[string]any) *ToolResult {
	localFile := getStr(args, "local_file")
	remotePath := getStr(args, "path")
	if localFile == "" {
		return ErrorResult("local_file is required for file_upload")
	}
	if remotePath == "" {
		remotePath = "/public_html"
	}
	if err := validateRemotePath(remotePath); err != nil {
		return ErrorResult(fmt.Sprintf("invalid path: %v", err))
	}

	f, err := os.Open(localFile)
	if err != nil {
		return ErrorResult(fmt.Sprintf("cannot open local file: %v", err))
	}
	defer f.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	if err := writer.WriteField("dir", remotePath); err != nil {
		return ErrorResult(fmt.Sprintf("write form field: %v", err))
	}

	part, err := writer.CreateFormFile("file-0", filepath.Base(localFile))
	if err != nil {
		return ErrorResult(fmt.Sprintf("create form file: %v", err))
	}
	if _, err := io.Copy(part, f); err != nil {
		return ErrorResult(fmt.Sprintf("copy file data: %v", err))
	}
	if err := writer.Close(); err != nil {
		return ErrorResult(fmt.Sprintf("finalize upload: %v", err))
	}

	reqURL := t.uapiURL("Fileman", "upload_files")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, &buf)
	if err != nil {
		return ErrorResult(fmt.Sprintf("create request: %v", err))
	}
	req.Header.Set("Authorization", fmt.Sprintf("cpanel %s:%s", t.username, t.apiToken))
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := t.client.Do(req)
	if err != nil {
		return RetryableError(fmt.Sprintf("upload failed: %v", err), "Check network or cPanel host")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return ErrorResult(fmt.Sprintf("read upload response: %v", err))
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		if resp.StatusCode == http.StatusOK {
			return NewToolResult(fmt.Sprintf("**Uploaded** %s to %s", filepath.Base(localFile), remotePath))
		}
		return ErrorResult(fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncateStr(string(body), 200)))
	}

	if _, err := uapiOK(result); err != nil {
		return ErrorResult(fmt.Sprintf("Upload error: %v", err))
	}

	return NewToolResult(fmt.Sprintf("**Uploaded** `%s` → `%s/%s`", filepath.Base(localFile), remotePath, filepath.Base(localFile)))
}

func (t *CpanelTool) fileList(ctx context.Context, args map[string]any) *ToolResult {
	dir := getStr(args, "path")
	if dir == "" {
		dir = "/public_html"
	}
	if err := validateRemotePath(dir); err != nil {
		return ErrorResult(fmt.Sprintf("invalid path: %v", err))
	}

	params := url.Values{}
	params.Set("dir", dir)
	params.Set("include_mime", "1")
	params.Set("include_hash", "0")
	params.Set("include_permissions", "1")

	result, err := t.doGet(ctx, "Fileman", "list_files", params)
	if err != nil {
		return RetryableError(fmt.Sprintf("list files failed: %v", err), "Check cPanel connection")
	}

	data, err := uapiOK(result)
	if err != nil {
		return ErrorResult(fmt.Sprintf("list files: %v", err))
	}

	files, _ := data.([]any)
	if len(files) == 0 {
		return NewToolResult(fmt.Sprintf("Directory `%s` is empty.", dir))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**Files in `%s`** (%d items):\n\n", dir, len(files)))
	sb.WriteString("| Type | Name | Size | Modified |\n")
	sb.WriteString("|------|------|------|----------|\n")

	for _, f := range files {
		fm, ok := f.(map[string]any)
		if !ok {
			continue
		}
		name, _ := fm["file"].(string)
		ftype, _ := fm["type"].(string)
		size, _ := fm["humansize"].(string)
		mtime, _ := fm["mtime"].(float64)

		icon := "📄"
		if ftype == "dir" {
			icon = "📁"
		}
		timeStr := ""
		if mtime > 0 {
			timeStr = time.Unix(int64(mtime), 0).Format("2006-01-02 15:04")
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", icon, name, size, timeStr))
	}

	return NewToolResult(sb.String())
}

func (t *CpanelTool) fileDelete(ctx context.Context, args map[string]any) *ToolResult {
	path := getStr(args, "path")
	if path == "" {
		return ErrorResult("path is required for file_delete")
	}
	if err := validateRemotePath(path); err != nil {
		return ErrorResult(fmt.Sprintf("invalid path: %v", err))
	}

	dir := filepath.Dir(path)
	file := filepath.Base(path)

	params := url.Values{}
	params.Set("dir", dir)
	params.Set("files", file)

	result, err := t.doPost(ctx, "Fileman", "trash", params)
	if err != nil {
		return ErrorResult(fmt.Sprintf("delete failed: %v", err))
	}

	if _, err := uapiOK(result); err != nil {
		return ErrorResult(fmt.Sprintf("delete: %v", err))
	}

	return NewToolResult(fmt.Sprintf("**Deleted** `%s`", path))
}

func (t *CpanelTool) fileCreateDir(ctx context.Context, args map[string]any) *ToolResult {
	path := getStr(args, "path")
	if path == "" {
		return ErrorResult("path is required for file_create_dir")
	}
	if err := validateRemotePath(path); err != nil {
		return ErrorResult(fmt.Sprintf("invalid path: %v", err))
	}

	dir := filepath.Dir(path)
	name := filepath.Base(path)

	params := url.Values{}
	params.Set("dir", dir)
	params.Set("name", name)

	result, err := t.doPost(ctx, "Fileman", "mkdir", params)
	if err != nil {
		return ErrorResult(fmt.Sprintf("mkdir failed: %v", err))
	}

	if _, err := uapiOK(result); err != nil {
		return ErrorResult(fmt.Sprintf("mkdir: %v", err))
	}

	return NewToolResult(fmt.Sprintf("**Created directory** `%s`", path))
}

// ── Domain management ────────────────────────────────────────────────

func (t *CpanelTool) domainList(ctx context.Context) *ToolResult {
	result, err := t.doGet(ctx, "DomainInfo", "list_domains", nil)
	if err != nil {
		return RetryableError(fmt.Sprintf("domain list failed: %v", err), "Check cPanel connection")
	}

	data, err := uapiOK(result)
	if err != nil {
		return ErrorResult(fmt.Sprintf("domain list: %v", err))
	}

	dm, _ := data.(map[string]any)
	var sb strings.Builder
	sb.WriteString("**Domains:**\n\n")

	if main, ok := dm["main_domain"].(string); ok {
		sb.WriteString(fmt.Sprintf("**Main domain:** %s\n\n", main))
	}

	if addons, ok := dm["addon_domains"].([]any); ok && len(addons) > 0 {
		sb.WriteString("**Addon domains:**\n")
		for _, d := range addons {
			if s, ok := d.(string); ok {
				sb.WriteString(fmt.Sprintf("- %s\n", s))
			}
		}
		sb.WriteString("\n")
	}

	if subs, ok := dm["sub_domains"].([]any); ok && len(subs) > 0 {
		sb.WriteString("**Subdomains:**\n")
		for _, d := range subs {
			if s, ok := d.(string); ok {
				sb.WriteString(fmt.Sprintf("- %s\n", s))
			}
		}
		sb.WriteString("\n")
	}

	if parked, ok := dm["parked_domains"].([]any); ok && len(parked) > 0 {
		sb.WriteString("**Parked domains:**\n")
		for _, d := range parked {
			if s, ok := d.(string); ok {
				sb.WriteString(fmt.Sprintf("- %s\n", s))
			}
		}
	}

	return NewToolResult(sb.String())
}

func (t *CpanelTool) domainAddAddon(ctx context.Context, args map[string]any) *ToolResult {
	domain := getStr(args, "domain")
	if domain == "" {
		return ErrorResult("domain is required for domain_add_addon")
	}

	docRoot := getStr(args, "document_root")
	if docRoot == "" {
		docRoot = "/" + domain
	}

	subdomain := getStr(args, "subdomain")
	if subdomain == "" {
		subdomain = strings.Split(domain, ".")[0]
	}

	params := url.Values{}
	params.Set("newdomain", domain)
	params.Set("subdomain", subdomain)
	params.Set("dir", docRoot)

	result, err := t.doPost(ctx, "AddonDomain", "addaddondomain", params)
	if err != nil {
		return ErrorResult(fmt.Sprintf("add addon domain failed: %v", err))
	}

	if _, err := uapiOK(result); err != nil {
		return ErrorResult(fmt.Sprintf("add addon domain: %v", err))
	}

	return NewToolResult(fmt.Sprintf("**Addon domain added:** %s\nDocument root: `%s`", domain, docRoot))
}

func (t *CpanelTool) domainAddSub(ctx context.Context, args map[string]any) *ToolResult {
	subdomain := getStr(args, "subdomain")
	domain := getStr(args, "domain")
	if subdomain == "" {
		return ErrorResult("subdomain is required for domain_add_sub")
	}
	if domain == "" {
		return ErrorResult("domain is required for domain_add_sub (the root domain)")
	}

	docRoot := getStr(args, "document_root")
	if docRoot == "" {
		docRoot = fmt.Sprintf("/%s.%s", subdomain, domain)
	}

	params := url.Values{}
	params.Set("domain", subdomain)
	params.Set("rootdomain", domain)
	params.Set("dir", docRoot)

	result, err := t.doPost(ctx, "SubDomain", "addsubdomain", params)
	if err != nil {
		return ErrorResult(fmt.Sprintf("add subdomain failed: %v", err))
	}

	if _, err := uapiOK(result); err != nil {
		return ErrorResult(fmt.Sprintf("add subdomain: %v", err))
	}

	return NewToolResult(fmt.Sprintf("**Subdomain added:** %s.%s\nDocument root: `%s`", subdomain, domain, docRoot))
}

func (t *CpanelTool) domainRemove(ctx context.Context, args map[string]any) *ToolResult {
	domain := getStr(args, "domain")
	if domain == "" {
		return ErrorResult("domain is required for domain_remove")
	}

	// Try addon first, then subdomain
	params := url.Values{}
	params.Set("domain", domain)

	result, err := t.doPost(ctx, "AddonDomain", "deladdondomain", params)
	if err == nil {
		if _, uErr := uapiOK(result); uErr == nil {
			return NewToolResult(fmt.Sprintf("**Addon domain removed:** %s", domain))
		}
	}

	// Try as subdomain
	parts := strings.SplitN(domain, ".", 2)
	if len(parts) == 2 {
		params = url.Values{}
		params.Set("domain", parts[0]+"_"+parts[1])

		result, err = t.doPost(ctx, "SubDomain", "delsubdomain", params)
		if err == nil {
			if _, uErr := uapiOK(result); uErr == nil {
				return NewToolResult(fmt.Sprintf("**Subdomain removed:** %s", domain))
			}
		}
	}

	return ErrorResult(fmt.Sprintf("could not remove domain %s — it may be the main domain or not exist", domain))
}

func (t *CpanelTool) domainRedirects(ctx context.Context) *ToolResult {
	result, err := t.doGet(ctx, "Mime", "list_redirects", nil)
	if err != nil {
		return RetryableError(fmt.Sprintf("list redirects failed: %v", err), "Check cPanel connection")
	}

	data, err := uapiOK(result)
	if err != nil {
		return ErrorResult(fmt.Sprintf("list redirects: %v", err))
	}

	redirects, _ := data.([]any)
	if len(redirects) == 0 {
		return NewToolResult("No redirects configured.")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**Redirects (%d):**\n\n", len(redirects)))
	for i, r := range redirects {
		rm, ok := r.(map[string]any)
		if !ok {
			continue
		}
		src, _ := rm["source"].(string)
		dst, _ := rm["destination"].(string)
		rtype, _ := rm["type"].(string)
		sb.WriteString(fmt.Sprintf("%d. %s → %s (type: %s)\n", i+1, src, dst, rtype))
	}

	return NewToolResult(sb.String())
}

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
	sb.WriteString(fmt.Sprintf("**MySQL Databases (%d):**\n\n", len(dbs)))
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
		sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n", name, sizeStr, users))
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
	sb.WriteString(fmt.Sprintf("**MySQL Users (%d):**\n", len(users)))
	for i, u := range users {
		if s, ok := u.(string); ok {
			sb.WriteString(fmt.Sprintf("%d. `%s`\n", i+1, s))
		} else if m, ok := u.(map[string]any); ok {
			name, _ := m["user"].(string)
			sb.WriteString(fmt.Sprintf("%d. `%s`\n", i+1, name))
		}
	}

	return NewToolResult(sb.String())
}

// ── SSL management ───────────────────────────────────────────────────

func (t *CpanelTool) sslList(ctx context.Context) *ToolResult {
	result, err := t.doGet(ctx, "SSL", "list_certs", nil)
	if err != nil {
		return RetryableError(fmt.Sprintf("list SSL certs failed: %v", err), "Check cPanel connection")
	}

	data, err := uapiOK(result)
	if err != nil {
		return ErrorResult(fmt.Sprintf("list SSL certs: %v", err))
	}

	certs, _ := data.([]any)
	if len(certs) == 0 {
		return NewToolResult("No SSL certificates installed.")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**SSL Certificates (%d):**\n\n", len(certs)))
	for i, c := range certs {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		domain, _ := cm["domain"].(string)
		issuer, _ := cm["issuer.organizationName"].(string)
		notAfter, _ := cm["not_after"].(string)

		sb.WriteString(fmt.Sprintf("%d. **%s**\n", i+1, domain))
		if issuer != "" {
			sb.WriteString(fmt.Sprintf("   Issuer: %s\n", issuer))
		}
		if notAfter != "" {
			sb.WriteString(fmt.Sprintf("   Expires: %s\n", notAfter))
		}
		sb.WriteString("\n")
	}

	return NewToolResult(sb.String())
}

func (t *CpanelTool) sslInstall(ctx context.Context, args map[string]any) *ToolResult {
	domain := getStr(args, "domain")
	cert := getStr(args, "ssl_cert")
	key := getStr(args, "ssl_key")
	if domain == "" {
		return ErrorResult("domain is required for ssl_install")
	}
	if cert == "" {
		return ErrorResult("ssl_cert is required for ssl_install")
	}
	if key == "" {
		return ErrorResult("ssl_key is required for ssl_install")
	}

	params := url.Values{}
	params.Set("domain", domain)
	params.Set("cert", cert)
	params.Set("key", key)
	if ca := getStr(args, "ssl_cabundle"); ca != "" {
		params.Set("cabundle", ca)
	}

	result, err := t.doPost(ctx, "SSL", "install_ssl", params)
	if err != nil {
		return ErrorResult(fmt.Sprintf("install SSL failed: %v", err))
	}

	if _, err := uapiOK(result); err != nil {
		return ErrorResult(fmt.Sprintf("install SSL: %v", err))
	}

	return NewToolResult(fmt.Sprintf("**SSL installed** for %s", domain))
}

// ── Generic UAPI ─────────────────────────────────────────────────────

func (t *CpanelTool) uapiGeneric(ctx context.Context, args map[string]any) *ToolResult {
	module := getStr(args, "module")
	function := getStr(args, "function")
	if module == "" {
		return ErrorResult("module is required for uapi action (e.g. 'Email', 'Ftp', 'Backup')")
	}
	if function == "" {
		return ErrorResult("function is required for uapi action (e.g. 'list_pops', 'add_pop')")
	}

	params := url.Values{}
	if p, ok := args["params"].(map[string]any); ok {
		for k, v := range p {
			if s, ok := v.(string); ok {
				params.Set(k, s)
			} else {
				// Handle non-string values from JSON (numbers, booleans)
				params.Set(k, fmt.Sprintf("%v", v))
			}
		}
	}

	method := getStr(args, "method")

	var result map[string]any
	var err error
	if strings.EqualFold(method, "POST") {
		result, err = t.doPost(ctx, module, function, params)
	} else {
		result, err = t.doGet(ctx, module, function, params)
	}
	if err != nil {
		return RetryableError(
			fmt.Sprintf("UAPI %s/%s failed: %v", module, function, err),
			"Check cPanel connection or parameters",
		)
	}

	data, uErr := uapiOK(result)
	if uErr != nil {
		return ErrorResult(fmt.Sprintf("UAPI %s/%s: %v", module, function, uErr))
	}

	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return ErrorResult(fmt.Sprintf("marshal response: %v", err))
	}

	return NewToolResult(fmt.Sprintf("**UAPI %s/%s** response:\n```json\n%s\n```", module, function, string(out)))
}
