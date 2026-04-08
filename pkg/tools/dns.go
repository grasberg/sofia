package tools

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

// DNSTool provides DNS resolution and record lookup using Go's net package.
type DNSTool struct{}

func NewDNSTool() *DNSTool { return &DNSTool{} }

func (t *DNSTool) Name() string { return "dns" }
func (t *DNSTool) Description() string {
	return "DNS lookup and diagnostics. Resolve domain names and query DNS records (A, AAAA, MX, CNAME, TXT, NS, SRV). Also supports reverse DNS lookup."
}

func (t *DNSTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"domain": map[string]any{
				"type":        "string",
				"description": "Domain name to look up (e.g., example.com)",
			},
			"record_type": map[string]any{
				"type":        "string",
				"description": "DNS record type to query (default: all)",
				"enum":        []string{"A", "AAAA", "MX", "CNAME", "TXT", "NS", "SRV", "PTR", "all"},
			},
			"ip": map[string]any{
				"type":        "string",
				"description": "IP address for reverse DNS lookup (PTR record)",
			},
		},
	}
}

func (t *DNSTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	// Reverse lookup
	if ip, ok := args["ip"].(string); ok && ip != "" {
		return t.reverseLookup(ctx, ip)
	}

	domain, ok := args["domain"].(string)
	if !ok || domain == "" {
		return ErrorResult("domain or ip is required")
	}

	recordType := "all"
	if rt, ok := args["record_type"].(string); ok && rt != "" {
		recordType = strings.ToUpper(rt)
	}

	resolver := &net.Resolver{}
	dnsCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("DNS lookup for: %s\n\n", domain))

	if recordType == "all" || recordType == "A" || recordType == "AAAA" {
		ips, err := resolver.LookupHost(dnsCtx, domain)
		if err == nil && len(ips) > 0 {
			for _, ip := range ips {
				if strings.Contains(ip, ":") {
					sb.WriteString(fmt.Sprintf("AAAA  %s\n", ip))
				} else {
					sb.WriteString(fmt.Sprintf("A     %s\n", ip))
				}
			}
		} else if err != nil && recordType != "all" {
			sb.WriteString(fmt.Sprintf("A/AAAA: %v\n", err))
		}
	}

	if recordType == "all" || recordType == "MX" {
		mxs, err := resolver.LookupMX(dnsCtx, domain)
		if err == nil {
			for _, mx := range mxs {
				sb.WriteString(fmt.Sprintf("MX    %s (priority: %d)\n", mx.Host, mx.Pref))
			}
		}
	}

	if recordType == "all" || recordType == "CNAME" {
		cname, err := resolver.LookupCNAME(dnsCtx, domain)
		if err == nil && cname != "" {
			sb.WriteString(fmt.Sprintf("CNAME %s\n", cname))
		}
	}

	if recordType == "all" || recordType == "TXT" {
		txts, err := resolver.LookupTXT(dnsCtx, domain)
		if err == nil {
			for _, txt := range txts {
				sb.WriteString(fmt.Sprintf("TXT   %s\n", txt))
			}
		}
	}

	if recordType == "all" || recordType == "NS" {
		nss, err := resolver.LookupNS(dnsCtx, domain)
		if err == nil {
			for _, ns := range nss {
				sb.WriteString(fmt.Sprintf("NS    %s\n", ns.Host))
			}
		}
	}

	if recordType == "SRV" {
		_, srvs, err := resolver.LookupSRV(dnsCtx, "", "", domain)
		if err == nil {
			for _, srv := range srvs {
				sb.WriteString(fmt.Sprintf("SRV   %s:%d (priority: %d, weight: %d)\n",
					srv.Target, srv.Port, srv.Priority, srv.Weight))
			}
		}
	}

	output := sb.String()
	if strings.TrimSpace(output) == fmt.Sprintf("DNS lookup for: %s", domain) {
		return NewToolResult(fmt.Sprintf("No %s records found for %s", recordType, domain))
	}
	return NewToolResult(output)
}

func (t *DNSTool) reverseLookup(ctx context.Context, ip string) *ToolResult {
	dnsCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resolver := &net.Resolver{}
	names, err := resolver.LookupAddr(dnsCtx, ip)
	if err != nil {
		return ErrorResult(fmt.Sprintf("reverse lookup failed: %v", err))
	}

	if len(names) == 0 {
		return NewToolResult(fmt.Sprintf("No PTR records found for %s", ip))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Reverse DNS for: %s\n\n", ip))
	for _, name := range names {
		sb.WriteString(fmt.Sprintf("PTR   %s\n", name))
	}
	return NewToolResult(sb.String())
}
