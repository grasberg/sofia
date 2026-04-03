---
name: porkbun
description: "Manage domains via Porkbun: register domains, set nameservers, manage DNS records, check availability, and get pricing. Use when the user mentions Porkbun, domain registration, nameservers, or DNS management for domains registered at Porkbun."
---

# Porkbun Domain Management

Use the `domain_name` tool to manage domains at Porkbun.

## Available Actions

| Action | Description |
|--------|-------------|
| `check` | Check domain availability and price |
| `register` | Register (buy) a domain |
| `list` | List all owned domains |
| `pricing` | Show TLD pricing |
| `get_nameservers` | Get current nameservers for a domain |
| `update_nameservers` | Set custom nameservers |
| `dns_list` | List DNS records for a domain |
| `dns_create` | Add a DNS record |
| `dns_delete` | Delete a DNS record |

## Common Workflows

### Register a domain and point it to hosting

1. Check availability:
```
domain_name(action="check", domain="example.se")
```

2. Register if available:
```
domain_name(action="register", domain="example.se")
```

3. Set nameservers to the hosting provider (e.g. cPanel):
```
domain_name(action="update_nameservers", domain="example.se",
            nameservers=["ns1.hostingprovider.com", "ns2.hostingprovider.com"])
```

### Common nameserver values

- **Porkbun default:** `curitiba.ns.porkbun.com`, `fortaleza.ns.porkbun.com`, `maceio.ns.porkbun.com`, `salvador.ns.porkbun.com`
- **Cloudflare:** Ask the user — Cloudflare assigns unique nameservers per account
- **Custom hosting:** Ask the user for their hosting provider's nameservers

### Manage DNS records (when using Porkbun nameservers)

DNS records can only be managed via Porkbun when the domain uses Porkbun's nameservers. If nameservers point elsewhere, manage DNS at that provider instead.

Add an A record:
```
domain_name(action="dns_create", domain="example.se",
            record_type="A", record_name="", record_content="1.2.3.4")
```

Add a CNAME:
```
domain_name(action="dns_create", domain="example.se",
            record_type="CNAME", record_name="www", record_content="example.se")
```

Add a TXT record (e.g. SPF):
```
domain_name(action="dns_create", domain="example.se",
            record_type="TXT", record_name="", record_content="v=spf1 include:_spf.google.com ~all")
```

List current records:
```
domain_name(action="dns_list", domain="example.se")
```

Delete a record (get ID from dns_list first):
```
domain_name(action="dns_delete", domain="example.se", record_id="123456789")
```

## Important Notes

- After changing nameservers, DNS propagation can take up to 24-48 hours.
- If the user wants to use the domain with cPanel hosting, set nameservers to the hosting provider's NS and then add the domain as an addon domain in cPanel.
- If the user wants to manage DNS at Porkbun (e.g. for Cloudflare, Vercel, or other services that use CNAME/A records), keep Porkbun's default nameservers and add the appropriate DNS records.
