package tools

import (
	"context"
	"fmt"
	"strings"
)

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
	fmt.Fprintf(&sb, "**DNS Records for %s (%d):**\n\n", domain, len(records))
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
		fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s | %s |\n", id, rType, name, content, ttl, prio)
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
	fmt.Fprintf(&sb, "**Nameservers for %s:**\n", domain)
	for i, n := range ns {
		if s, ok := n.(string); ok {
			fmt.Fprintf(&sb, "%d. %s\n", i+1, s)
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
