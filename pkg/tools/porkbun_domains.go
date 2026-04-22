package tools

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
)

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
	fmt.Fprintf(&sb, "**Domain:** %s\n", domain)
	if avail {
		sb.WriteString("**Available:** Yes\n")
	} else {
		sb.WriteString("**Available:** No\n")
	}

	if price, ok := resp["price"].(string); ok {
		fmt.Fprintf(&sb, "**Registration price:** $%s\n", price)
	}
	if regularPrice, ok := resp["regularPrice"].(string); ok {
		fmt.Fprintf(&sb, "**Regular price:** $%s\n", regularPrice)
	}
	if promo, ok := resp["firstYearPromo"].(string); ok && strings.EqualFold(promo, "yes") {
		sb.WriteString("**First year promo:** Yes\n")
	}

	// Renewal/transfer info under "additional"
	if additional, ok := resp["additional"].(map[string]any); ok {
		if renewal, ok := additional["renewal"].(map[string]any); ok {
			if renewPrice, ok := renewal["price"].(string); ok {
				fmt.Fprintf(&sb, "**Renewal:** $%s/yr\n", renewPrice)
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
	fmt.Fprintf(&sb, "**Domains (%d):**\n\n", len(domains))

	for i, d := range domains {
		dm, ok := d.(map[string]any)
		if !ok {
			continue
		}
		name, _ := dm["domain"].(string)
		expiry, _ := dm["expireDate"].(string)
		autoRenew, _ := dm["autoRenew"].(bool)

		fmt.Fprintf(&sb, "%d. **%s**\n", i+1, name)
		if expiry != "" {
			fmt.Fprintf(&sb, "   Expires: %s\n", expiry)
		}
		fmt.Fprintf(&sb, "   Auto-renew: %v\n", autoRenew)
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
			fmt.Fprintf(&sb, "| .%s | $%s | $%s |\n", tld, reg, renew)
		}
	}

	fmt.Fprintf(&sb, "\n*%d TLDs available total. Use check action for specific domain pricing.*", len(pricing))

	return NewToolResult(sb.String())
}
