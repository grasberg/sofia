---
name: cpanel
description: "Manage cPanel hosting accounts via the UAPI. Use for any cPanel task: domains, email accounts, databases, FTP, DNS zones, SSL certificates, file management, backups, cron jobs, and more. Trigger when the user mentions cPanel, hosting, webhosting, email accounts, or server management on a shared host."
---

# cPanel UAPI Skill

Use the `cpanel` tool to manage hosting accounts. The tool has built-in actions for common tasks and a generic `uapi` action for calling any UAPI endpoint.

## Built-in Actions

Use these for the most common operations:

| Action | What it does |
|--------|-------------|
| `file_upload` | Upload a local file to cPanel |
| `file_list` | List files in a directory |
| `file_delete` | Delete a file |
| `file_create_dir` | Create a directory |
| `domain_list` | List all domains |
| `domain_add_addon` | Add an addon domain |
| `domain_add_sub` | Add a subdomain |
| `domain_remove` | Remove a domain |
| `domain_redirects` | List redirects |
| `db_list` | List MySQL databases |
| `db_create` | Create a database |
| `db_delete` | Delete a database |
| `db_create_user` | Create a database user |
| `db_set_privileges` | Set user privileges on a database |
| `db_list_users` | List database users |
| `ssl_list` | List SSL certificates |
| `ssl_install` | Install an SSL certificate |

## Generic UAPI Action

For anything not covered above, use `action: "uapi"` with `module`, `function`, and optionally `params` and `method`.

```
cpanel(action="uapi", module="Email", function="list_pops", method="GET")
cpanel(action="uapi", module="Email", function="add_pop", method="POST", params={"email": "info", "domain": "example.com", "password": "secret123", "quota": "1024"})
```

Use `method: "POST"` for any call that creates, modifies, or deletes something. Use `method: "GET"` (default) for read-only calls.

## UAPI Module Reference

For detailed module/function listings, read the appropriate reference file:

- **Email** (accounts, forwarders, autoresponders): [references/email.md](references/email.md)
- **Domains & DNS** (zones, records, subdomains, parked): [references/domains-dns.md](references/domains-dns.md)
- **Databases** (MySQL, PostgreSQL): [references/databases.md](references/databases.md)
- **Files & FTP** (file manager, FTP accounts): [references/files-ftp.md](references/files-ftp.md)
- **SSL & Security** (certificates, SSH keys, ModSecurity): [references/ssl-security.md](references/ssl-security.md)
- **Backups, Cron & System** (backups, cron, resource usage, PHP): [references/system.md](references/system.md)

Read only the relevant reference when you need details about a specific module.
