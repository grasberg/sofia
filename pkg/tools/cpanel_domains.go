package tools

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

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
		fmt.Fprintf(&sb, "**Main domain:** %s\n\n", main)
	}

	if addons, ok := dm["addon_domains"].([]any); ok && len(addons) > 0 {
		sb.WriteString("**Addon domains:**\n")
		for _, d := range addons {
			if s, ok := d.(string); ok {
				fmt.Fprintf(&sb, "- %s\n", s)
			}
		}
		sb.WriteString("\n")
	}

	if subs, ok := dm["sub_domains"].([]any); ok && len(subs) > 0 {
		sb.WriteString("**Subdomains:**\n")
		for _, d := range subs {
			if s, ok := d.(string); ok {
				fmt.Fprintf(&sb, "- %s\n", s)
			}
		}
		sb.WriteString("\n")
	}

	if parked, ok := dm["parked_domains"].([]any); ok && len(parked) > 0 {
		sb.WriteString("**Parked domains:**\n")
		for _, d := range parked {
			if s, ok := d.(string); ok {
				fmt.Fprintf(&sb, "- %s\n", s)
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
	fmt.Fprintf(&sb, "**Redirects (%d):**\n\n", len(redirects))
	for i, r := range redirects {
		rm, ok := r.(map[string]any)
		if !ok {
			continue
		}
		src, _ := rm["source"].(string)
		dst, _ := rm["destination"].(string)
		rtype, _ := rm["type"].(string)
		fmt.Fprintf(&sb, "%d. %s → %s (type: %s)\n", i+1, src, dst, rtype)
	}

	return NewToolResult(sb.String())
}
