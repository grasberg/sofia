# Email Module Reference

## Email — Email Accounts

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `list_pops` | GET | — | List all email accounts |
| `list_pops_with_disk` | GET | — | List email accounts with disk usage |
| `add_pop` | POST | `email`, `password`, `quota` (MB, 0=unlimited), `domain` | Create email account |
| `delete_pop` | POST | `email`, `domain` | Delete email account |
| `edit_pop_quota` | POST | `email`, `domain`, `quota` | Change mailbox quota |
| `passwd_pop` | POST | `email`, `domain`, `password` | Change email password |
| `get_pop_quota` | GET | `email`, `domain` | Get quota for an account |
| `get_disk_usage` | GET | `email`, `domain` | Get disk usage for an account |

### Example: Create email account

```
cpanel(action="uapi", module="Email", function="add_pop", method="POST",
       params={"email": "info", "domain": "example.com", "password": "Str0ngP@ss!", "quota": "2048"})
```

### Example: List all email accounts

```
cpanel(action="uapi", module="Email", function="list_pops_with_disk")
```

## Email — Forwarders

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `list_forwarders` | GET | `domain` | List forwarders for a domain |
| `add_forwarder` | POST | `domain`, `email` (local part), `fwdopt` ("fwd"), `fwdemail` (destination) | Create forwarder |
| `delete_forwarder` | POST | `address` (full address), `forwarder` (destination) | Delete forwarder |

### Example: Forward info@example.com to admin@gmail.com

```
cpanel(action="uapi", module="Email", function="add_forwarder", method="POST",
       params={"domain": "example.com", "email": "info", "fwdopt": "fwd", "fwdemail": "admin@gmail.com"})
```

## Email — Autoresponders

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `list_auto_responders` | GET | `domain` | List autoresponders |
| `add_auto_responder` | POST | `email`, `domain`, `from`, `subject`, `body`, `interval` (hours) | Create autoresponder |
| `delete_auto_responder` | POST | `email`, `domain` | Delete autoresponder |

## Email — Spam Filters (SpamAssassin)

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `get_spam_settings` | GET | — | Get SpamAssassin settings |
| `enable_spam_assassin` | POST | — | Enable SpamAssassin |
| `disable_spam_assassin` | POST | — | Disable SpamAssassin |

## Email — DKIM/SPF

Module: `DKIM`

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `ensure_valid_dkim` | POST | `domain` | Generate/validate DKIM for domain |

Module: `EmailAuth`

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `validate_current_spfs` | GET | — | Validate SPF records |
| `validate_current_dkims` | GET | — | Validate DKIM records |
