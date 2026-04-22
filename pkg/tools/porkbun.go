package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const porkbunAPI = "https://api.porkbun.com/api/json/v3"

// PorkbunTool manages domain names via the Porkbun API.
type PorkbunTool struct {
	apiKey       string
	secretAPIKey string
	client       *http.Client
}

// NewPorkbunTool creates a new Porkbun domain management tool.
func NewPorkbunTool(apiKey, secretAPIKey string) *PorkbunTool {
	return &PorkbunTool{
		apiKey:       apiKey,
		secretAPIKey: secretAPIKey,
		client:       &http.Client{Timeout: 30 * time.Second},
	}
}

func (t *PorkbunTool) Name() string {
	return "domain_name"
}

func (t *PorkbunTool) Description() string {
	return "Manage domain names via Porkbun: check availability, register domains, list owned domains, " +
		"manage DNS records, and get pricing. Supports actions: check, register, list, pricing, " +
		"dns_list, dns_create, dns_delete, get_nameservers, update_nameservers."
}

func (t *PorkbunTool) Parameters() map[string]any {
	var schema map[string]any
	_ = json.Unmarshal([]byte(`{
		"type": "object",
		"properties": {
			"action": {
				"type": "string",
				"enum": ["check", "register", "list", "pricing", "dns_list", "dns_create", "dns_delete", "get_nameservers", "update_nameservers"],
				"description": "Action to perform: check (availability), register (buy domain), list (owned domains), pricing (TLD prices), dns_list (list records), dns_create (add record), dns_delete (remove record), get_nameservers, update_nameservers"
			},
			"domain": {
				"type": "string",
				"description": "Domain name (e.g. example.com). Required for check, register, dns_*, *_nameservers."
			},
			"record_type": {
				"type": "string",
				"description": "DNS record type: A, AAAA, CNAME, MX, TXT, NS, SRV, CAA, ALIAS, HTTPS, SVCB, TLSA, SSHFP"
			},
			"record_name": {
				"type": "string",
				"description": "DNS record name/subdomain (e.g. 'www' or '' for root)"
			},
			"record_content": {
				"type": "string",
				"description": "DNS record content/value (e.g. IP address, CNAME target)"
			},
			"record_ttl": {
				"type": "integer",
				"description": "DNS record TTL in seconds (default: 600)"
			},
			"record_prio": {
				"type": "integer",
				"description": "DNS record priority (for MX, SRV)"
			},
			"record_id": {
				"type": "string",
				"description": "DNS record ID (required for dns_delete)"
			},
			"nameservers": {
				"type": "array",
				"items": {"type": "string"},
				"description": "List of nameservers (for update_nameservers)"
			}
		},
		"required": ["action"]
	}`), &schema)
	return schema
}

func (t *PorkbunTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	action, _ := args["action"].(string)
	domain, _ := args["domain"].(string)

	switch action {
	case "check":
		return t.checkDomain(ctx, domain)
	case "register":
		return t.registerDomain(ctx, domain)
	case "list":
		return t.listDomains(ctx)
	case "pricing":
		return t.getPricing(ctx)
	case "dns_list":
		return t.dnsListRecords(ctx, domain)
	case "dns_create":
		return t.dnsCreateRecord(ctx, domain, args)
	case "dns_delete":
		return t.dnsDeleteRecord(ctx, domain, args)
	case "get_nameservers":
		return t.getNameservers(ctx, domain)
	case "update_nameservers":
		return t.updateNameservers(ctx, domain, args)
	default:
		return ErrorResult(fmt.Sprintf("unknown action %q — use: check, register, list, pricing, "+
			"dns_list, dns_create, dns_delete, get_nameservers, update_nameservers", action))
	}
}

// ── API helpers ──────────────────────────────────────────────────────

func (t *PorkbunTool) authBody() map[string]any {
	return map[string]any{
		"apikey":       t.apiKey,
		"secretapikey": t.secretAPIKey,
	}
}

func porkbunPath(parts ...string) string {
	var b strings.Builder
	for _, p := range parts {
		b.WriteString("/")
		b.WriteString(url.PathEscape(p))
	}
	return b.String()
}

func (t *PorkbunTool) post(ctx context.Context, path string, body map[string]any) (map[string]any, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, porkbunAPI+path, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w (body: %s)", err, truncateStr(string(respBody), 200))
	}

	return result, nil
}
