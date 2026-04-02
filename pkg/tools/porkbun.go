package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
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

// ── Actions ──────────────────────────────────────────────────────────

func (t *PorkbunTool) checkDomain(ctx context.Context, domain string) *ToolResult {
	if domain == "" {
		return ErrorResult("domain is required for check action")
	}

	result, err := t.post(ctx, "/domain/checkDomain"+porkbunPath(domain), t.authBody())
	if err != nil {
		return RetryableError(fmt.Sprintf("Porkbun check failed: %v", err), "Check network or try again")
	}

	status, _ := result["status"].(string)
	if status != "SUCCESS" {
		msg, _ := result["message"].(string)
		return ErrorResult(fmt.Sprintf("Porkbun error: %s", msg))
	}

	// Response data is nested under "response" key
	resp, _ := result["response"].(map[string]any)
	if resp == nil {
		// Fallback: some endpoints may return fields at top level
		resp = result
	}

	// "avail" is a string ("yes"/"no"), not a boolean
	availStr, _ := resp["avail"].(string)
	avail := strings.EqualFold(availStr, "yes")

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**Domain:** %s\n", domain))
	if avail {
		sb.WriteString("**Available:** Yes\n")
	} else {
		sb.WriteString("**Available:** No\n")
	}

	if price, ok := resp["price"].(string); ok {
		sb.WriteString(fmt.Sprintf("**Registration price:** $%s\n", price))
	}
	if regularPrice, ok := resp["regularPrice"].(string); ok {
		sb.WriteString(fmt.Sprintf("**Regular price:** $%s\n", regularPrice))
	}
	if promo, ok := resp["firstYearPromo"].(string); ok && strings.EqualFold(promo, "yes") {
		sb.WriteString("**First year promo:** Yes\n")
	}

	// Renewal/transfer info under "additional"
	if additional, ok := resp["additional"].(map[string]any); ok {
		if renewal, ok := additional["renewal"].(map[string]any); ok {
			if renewPrice, ok := renewal["price"].(string); ok {
				sb.WriteString(fmt.Sprintf("**Renewal:** $%s/yr\n", renewPrice))
			}
		}
	}

	return NewToolResult(sb.String())
}

func (t *PorkbunTool) registerDomain(ctx context.Context, domain string) *ToolResult {
	if domain == "" {
		return ErrorResult("domain is required for register action")
	}

	// Step 1: Check availability and get the price
	checkResult, err := t.post(ctx, "/domain/checkDomain"+porkbunPath(domain), t.authBody())
	if err != nil {
		return RetryableError(fmt.Sprintf("Porkbun price check failed: %v", err), "Check network or try again")
	}

	checkStatus, _ := checkResult["status"].(string)
	if checkStatus != "SUCCESS" {
		msg, _ := checkResult["message"].(string)
		return ErrorResult(fmt.Sprintf("Price check failed: %s", msg))
	}

	resp, _ := checkResult["response"].(map[string]any)
	if resp == nil {
		resp = checkResult
	}

	availStr, _ := resp["avail"].(string)
	if !strings.EqualFold(availStr, "yes") {
		return ErrorResult(fmt.Sprintf("Domain %s is not available for registration.", domain))
	}

	priceStr, _ := resp["price"].(string)
	if priceStr == "" {
		return ErrorResult("Could not determine registration price for " + domain)
	}

	// Convert price from dollars (string, e.g. "9.73") to pennies (int, e.g. 973).
	// The Porkbun API returns price as a dollar string but expects cost as pennies integer.
	priceDollars, err := strconv.ParseFloat(priceStr, 64)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Invalid price %q from Porkbun: %v", priceStr, err))
	}
	costPennies := int(math.Round(priceDollars * 100))

	// Step 2: Register with required cost (pennies) and agreeToTerms fields
	body := t.authBody()
	body["cost"] = costPennies
	body["agreeToTerms"] = "yes"

	result, err := t.post(ctx, "/domain/create"+porkbunPath(domain), body)
	if err != nil {
		return RetryableError(fmt.Sprintf("Porkbun register failed: %v", err), "Check network or try again")
	}

	status, _ := result["status"].(string)
	if status != "SUCCESS" {
		msg, _ := result["message"].(string)
		return ErrorResult(fmt.Sprintf("Registration failed: %s", msg))
	}

	return NewToolResult(fmt.Sprintf(
		"**Domain registered:** %s\nPrice: $%s\nRegistration successful!", domain, priceStr,
	))
}

func (t *PorkbunTool) listDomains(ctx context.Context) *ToolResult {
	body := t.authBody()
	body["start"] = 0
	body["includeLabels"] = "yes"

	result, err := t.post(ctx, "/domain/listAll", body)
	if err != nil {
		return RetryableError(fmt.Sprintf("Porkbun list failed: %v", err), "Check network or try again")
	}

	status, _ := result["status"].(string)
	if status != "SUCCESS" {
		msg, _ := result["message"].(string)
		return ErrorResult(fmt.Sprintf("List domains failed: %s", msg))
	}

	domains, _ := result["domains"].([]any)
	if len(domains) == 0 {
		return NewToolResult("No domains found in your Porkbun account.")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**Domains (%d):**\n\n", len(domains)))

	for i, d := range domains {
		dm, ok := d.(map[string]any)
		if !ok {
			continue
		}
		name, _ := dm["domain"].(string)
		expiry, _ := dm["expireDate"].(string)
		autoRenew, _ := dm["autoRenew"].(bool)

		sb.WriteString(fmt.Sprintf("%d. **%s**\n", i+1, name))
		if expiry != "" {
			sb.WriteString(fmt.Sprintf("   Expires: %s\n", expiry))
		}
		sb.WriteString(fmt.Sprintf("   Auto-renew: %v\n", autoRenew))
		sb.WriteString("\n")
	}

	return NewToolResult(sb.String())
}

func (t *PorkbunTool) getPricing(ctx context.Context) *ToolResult {
	// Pricing endpoint doesn't require auth
	result, err := t.post(ctx, "/pricing/get", map[string]any{})
	if err != nil {
		return RetryableError(fmt.Sprintf("Porkbun pricing failed: %v", err), "Check network or try again")
	}

	status, _ := result["status"].(string)
	if status != "SUCCESS" {
		msg, _ := result["message"].(string)
		return ErrorResult(fmt.Sprintf("Pricing failed: %s", msg))
	}

	pricing, _ := result["pricing"].(map[string]any)
	if len(pricing) == 0 {
		return NewToolResult("No pricing data available.")
	}

	// Show popular TLDs only to keep output manageable
	popular := []string{"com", "net", "org", "io", "dev", "app", "se", "co", "ai", "xyz", "me", "info", "tech", "cloud"}
	var sb strings.Builder
	sb.WriteString("**Domain Pricing (popular TLDs):**\n\n")
	sb.WriteString("| TLD | Registration | Renewal |\n")
	sb.WriteString("|-----|-------------|----------|\n")

	for _, tld := range popular {
		if p, ok := pricing[tld].(map[string]any); ok {
			reg, _ := p["registration"].(string)
			renew, _ := p["renewal"].(string)
			sb.WriteString(fmt.Sprintf("| .%s | $%s | $%s |\n", tld, reg, renew))
		}
	}

	sb.WriteString(
		fmt.Sprintf("\n*%d TLDs available total. Use check action for specific domain pricing.*", len(pricing)),
	)

	return NewToolResult(sb.String())
}

func (t *PorkbunTool) dnsListRecords(ctx context.Context, domain string) *ToolResult {
	if domain == "" {
		return ErrorResult("domain is required for dns_list action")
	}

	result, err := t.post(ctx, "/dns/retrieve"+porkbunPath(domain), t.authBody())
	if err != nil {
		return RetryableError(fmt.Sprintf("DNS list failed: %v", err), "Check network or try again")
	}

	status, _ := result["status"].(string)
	if status != "SUCCESS" {
		msg, _ := result["message"].(string)
		return ErrorResult(fmt.Sprintf("DNS list failed: %s", msg))
	}

	records, _ := result["records"].([]any)
	if len(records) == 0 {
		return NewToolResult(fmt.Sprintf("No DNS records found for %s.", domain))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**DNS Records for %s (%d):**\n\n", domain, len(records)))
	sb.WriteString("| ID | Type | Name | Content | TTL | Prio |\n")
	sb.WriteString("|-----|------|------|---------|-----|------|\n")

	for _, r := range records {
		rec, ok := r.(map[string]any)
		if !ok {
			continue
		}
		id, _ := rec["id"].(string)
		rType, _ := rec["type"].(string)
		name, _ := rec["name"].(string)
		content, _ := rec["content"].(string)
		ttl, _ := rec["ttl"].(string)
		prio, _ := rec["prio"].(string)

		// Truncate long content
		if len(content) > 50 {
			content = content[:47] + "..."
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s |\n", id, rType, name, content, ttl, prio))
	}

	return NewToolResult(sb.String())
}

func (t *PorkbunTool) dnsCreateRecord(ctx context.Context, domain string, args map[string]any) *ToolResult {
	if domain == "" {
		return ErrorResult("domain is required for dns_create action")
	}

	recordType, _ := args["record_type"].(string)
	if recordType == "" {
		return ErrorResult("record_type is required (e.g. A, CNAME, MX, TXT)")
	}
	recordContent, _ := args["record_content"].(string)
	if recordContent == "" {
		return ErrorResult("record_content is required")
	}

	body := t.authBody()
	body["type"] = recordType
	body["content"] = recordContent

	if name, ok := args["record_name"].(string); ok {
		body["name"] = name
	}
	if ttl, ok := args["record_ttl"].(float64); ok && ttl > 0 {
		body["ttl"] = fmt.Sprintf("%d", int(ttl))
	} else {
		body["ttl"] = "600"
	}
	if prio, ok := args["record_prio"].(float64); ok {
		body["prio"] = fmt.Sprintf("%d", int(prio))
	}

	result, err := t.post(ctx, "/dns/create"+porkbunPath(domain), body)
	if err != nil {
		return RetryableError(fmt.Sprintf("DNS create failed: %v", err), "Check network or try again")
	}

	status, _ := result["status"].(string)
	if status != "SUCCESS" {
		msg, _ := result["message"].(string)
		return ErrorResult(fmt.Sprintf("DNS create failed: %s", msg))
	}

	id, _ := result["id"].(float64)
	return NewToolResult(
		fmt.Sprintf("**DNS record created** (ID: %.0f)\nType: %s, Content: %s", id, recordType, recordContent),
	)
}

func (t *PorkbunTool) dnsDeleteRecord(ctx context.Context, domain string, args map[string]any) *ToolResult {
	if domain == "" {
		return ErrorResult("domain is required for dns_delete action")
	}
	recordID, _ := args["record_id"].(string)
	if recordID == "" {
		return ErrorResult("record_id is required for dns_delete")
	}

	result, err := t.post(ctx, "/dns/delete"+porkbunPath(domain, recordID), t.authBody())
	if err != nil {
		return RetryableError(fmt.Sprintf("DNS delete failed: %v", err), "Check network or try again")
	}

	status, _ := result["status"].(string)
	if status != "SUCCESS" {
		msg, _ := result["message"].(string)
		return ErrorResult(fmt.Sprintf("DNS delete failed: %s", msg))
	}

	return NewToolResult(fmt.Sprintf("**DNS record %s deleted** from %s.", recordID, domain))
}

func (t *PorkbunTool) getNameservers(ctx context.Context, domain string) *ToolResult {
	if domain == "" {
		return ErrorResult("domain is required for get_nameservers action")
	}

	result, err := t.post(ctx, "/domain/getNs"+porkbunPath(domain), t.authBody())
	if err != nil {
		return RetryableError(fmt.Sprintf("Get nameservers failed: %v", err), "Check network or try again")
	}

	status, _ := result["status"].(string)
	if status != "SUCCESS" {
		msg, _ := result["message"].(string)
		return ErrorResult(fmt.Sprintf("Get nameservers failed: %s", msg))
	}

	ns, _ := result["ns"].([]any)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**Nameservers for %s:**\n", domain))
	for i, n := range ns {
		if s, ok := n.(string); ok {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, s))
		}
	}

	return NewToolResult(sb.String())
}

func (t *PorkbunTool) updateNameservers(ctx context.Context, domain string, args map[string]any) *ToolResult {
	if domain == "" {
		return ErrorResult("domain is required for update_nameservers action")
	}

	nsRaw, ok := args["nameservers"].([]any)
	if !ok || len(nsRaw) == 0 {
		return ErrorResult("nameservers array is required")
	}

	body := t.authBody()
	ns := make([]string, 0, len(nsRaw))
	for _, n := range nsRaw {
		if s, ok := n.(string); ok && s != "" {
			ns = append(ns, s)
		}
	}
	if len(ns) == 0 {
		return ErrorResult("no valid nameservers provided")
	}
	body["ns"] = ns

	result, err := t.post(ctx, "/domain/updateNs"+porkbunPath(domain), body)
	if err != nil {
		return RetryableError(fmt.Sprintf("Update nameservers failed: %v", err), "Check network or try again")
	}

	status, _ := result["status"].(string)
	if status != "SUCCESS" {
		msg, _ := result["message"].(string)
		return ErrorResult(fmt.Sprintf("Update nameservers failed: %s", msg))
	}

	return NewToolResult(fmt.Sprintf("**Nameservers updated** for %s:\n%s", domain, strings.Join(ns, "\n")))
}
