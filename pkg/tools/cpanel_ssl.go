package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

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
	fmt.Fprintf(&sb, "**SSL Certificates (%d):**\n\n", len(certs))
	for i, c := range certs {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		domain, _ := cm["domain"].(string)
		issuer, _ := cm["issuer.organizationName"].(string)
		notAfter, _ := cm["not_after"].(string)

		fmt.Fprintf(&sb, "%d. **%s**\n", i+1, domain)
		if issuer != "" {
			fmt.Fprintf(&sb, "   Issuer: %s\n", issuer)
		}
		if notAfter != "" {
			fmt.Fprintf(&sb, "   Expires: %s\n", notAfter)
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

// uapiNameRe validates UAPI module and function names: must start with a letter,
// followed by alphanumerics or underscores only. No path separators or dots allowed.
var uapiNameRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*$`)

func (t *CpanelTool) uapiGeneric(ctx context.Context, args map[string]any) *ToolResult {
	module := getStr(args, "module")
	function := getStr(args, "function")
	if module == "" {
		return ErrorResult("module is required for uapi action (e.g. 'Email', 'Ftp', 'Backup')")
	}
	if function == "" {
		return ErrorResult("function is required for uapi action (e.g. 'list_pops', 'add_pop')")
	}

	// Validate module and function names to prevent path traversal or injection.
	if !uapiNameRe.MatchString(module) {
		return ErrorResult(
			"invalid module name: must start with a letter and contain only alphanumeric characters or underscores",
		)
	}
	if !uapiNameRe.MatchString(function) {
		return ErrorResult(
			"invalid function name: must start with a letter and contain only alphanumeric characters or underscores",
		)
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
