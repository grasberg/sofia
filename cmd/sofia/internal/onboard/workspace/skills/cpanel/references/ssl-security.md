# SSL & Security Module Reference

## SSL — Certificates

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `list_certs` | GET | — | List installed certificates |
| `install_ssl` | POST | `domain`, `cert`, `key`, `cabundle` (optional) | Install SSL certificate |
| `delete_cert` | POST | `id` | Delete a certificate |
| `generate_csr` | POST | `domains`, `countryName`, `stateOrProvinceName`, `localityName`, `organizationName` | Generate CSR |
| `list_keys` | GET | — | List private keys |
| `fetch_best_for_domain` | GET | `domain` | Get best certificate for domain |

### Example: List certificates

```
cpanel(action="uapi", module="SSL", function="list_certs")
```

## SSL — AutoSSL (Let's Encrypt)

Module: `SSL`

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `get_autossl_check_schedule` | GET | — | Get AutoSSL check schedule |
| `get_autossl_pending_queue` | GET | — | Check pending AutoSSL requests |

Module: `LetsEncrypt` (if available)

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `request_ssl` | POST | `domains` (comma-separated) | Request Let's Encrypt certificate |

## SSH — SSH Access

Module: `SSH`

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `list_keys` | GET | — | List SSH keys |
| `add_key` | POST | `name`, `key` (public key content) | Import SSH public key |
| `delete_key` | POST | `name`, `type` ("rsa") | Delete SSH key |
| `authorize_key` | POST | `name` | Authorize a key |
| `deauthorize_key` | POST | `name` | Deauthorize a key |

## ModSecurity

Module: `ModSecurity`

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `has_modsecurity_installed` | GET | — | Check if ModSecurity is active |
| `enable_all_domains` | POST | — | Enable ModSecurity for all domains |
| `disable_all_domains` | POST | — | Disable ModSecurity for all domains |

## Hotlink Protection

Module: `HotlinkProtection`

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `is_enabled` | GET | — | Check if hotlink protection is on |
| `enable` | POST | `urls` (allowed URLs) | Enable hotlink protection |
| `disable` | POST | — | Disable hotlink protection |

## IP Blocker

Module: `BlockIP`

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `get_ips` | GET | — | List blocked IPs |
| `add_ip` | POST | `ip` | Block an IP |
| `remove_ip` | POST | `ip` | Unblock an IP |
