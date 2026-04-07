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

	return NewToolResult(
		fmt.Sprintf("**Uploaded** `%s` → `%s/%s`", filepath.Base(localFile), remotePath, filepath.Base(localFile)),
	)
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
	fmt.Fprintf(&sb, "**Files in `%s`** (%d items):\n\n", dir, len(files))
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
		fmt.Fprintf(&sb, "| %s | %s | %s | %s |\n", icon, name, size, timeStr)
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
