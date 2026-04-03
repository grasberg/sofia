# Domains & DNS Module Reference

## DomainInfo — Query Domains

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `list_domains` | GET | — | List all domains (main, addon, sub, parked) |
| `single_domain_data` | GET | `domain` | Get details for a specific domain |
| `domains_data` | GET | — | Get detailed data for all domains |

## SubDomain — Subdomains

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `addsubdomain` | POST | `domain` (subdomain prefix), `rootdomain`, `dir` (document root) | Create subdomain |
| `delsubdomain` | POST | `domain` (formatted as `sub_rootdomain.tld`) | Delete subdomain |

## AddonDomain — Addon Domains

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `addaddondomain` | POST | `newdomain`, `subdomain`, `dir` (document root) | Add addon domain |
| `deladdondomain` | POST | `domain` | Remove addon domain |

## Redirects

Module: `Mime`

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `list_redirects` | GET | — | List all redirects |
| `add_redirect` | POST | `domain`, `redirect`, `redirect_url`, `type` (permanent/temp) | Add redirect |
| `delete_redirect` | POST | `domain`, `redirect` | Remove redirect |

## DNS — Zone Editor

Module: `DNS`

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `parse_zone` | GET | `zone` (domain) | Get all DNS records for a zone |

Module: `ZoneEdit`

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `fetchzone_records` | GET | `domain` | Fetch DNS records |
| `add_zone_record` | POST | `domain`, `type` (A/AAAA/CNAME/MX/TXT/SRV/CAA), `name`, `address` or `cname` or `txtdata`, `ttl` | Add DNS record |
| `edit_zone_record` | POST | `domain`, `line` (record line number), + same fields as add | Edit DNS record |
| `remove_zone_record` | POST | `domain`, `line` (record line number) | Remove DNS record |

### Example: Add an A record

```
cpanel(action="uapi", module="ZoneEdit", function="add_zone_record", method="POST",
       params={"domain": "example.com", "type": "A", "name": "app.example.com.", "address": "1.2.3.4", "ttl": "3600"})
```

### Example: Add a TXT record (SPF/DMARC/verification)

```
cpanel(action="uapi", module="ZoneEdit", function="add_zone_record", method="POST",
       params={"domain": "example.com", "type": "TXT", "name": "example.com.", "txtdata": "v=spf1 include:_spf.google.com ~all", "ttl": "3600"})
```

### Example: Add an MX record

```
cpanel(action="uapi", module="ZoneEdit", function="add_zone_record", method="POST",
       params={"domain": "example.com", "type": "MX", "name": "example.com.", "exchange": "mail.example.com.", "preference": "10", "ttl": "3600"})
```

## Parked Domains (Aliases)

Module: `Park`

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `listparkeddomains` | GET | — | List parked domains |
| `park` | POST | `domain` | Park (alias) a domain |
| `unpark` | POST | `domain` | Unpark a domain |
