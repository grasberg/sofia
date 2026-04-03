# Databases Module Reference

## Mysql — MySQL Databases

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `list_databases` | GET | — | List all databases |
| `create_database` | POST | `name` | Create database (auto-prefixed with cPanel user) |
| `delete_database` | POST | `name` | Delete database |
| `list_users` | GET | — | List all database users |
| `create_user` | POST | `name`, `password` | Create database user |
| `delete_user` | POST | `name` | Delete database user |
| `set_privileges_on_database` | POST | `user`, `database`, `privileges` (e.g. "ALL PRIVILEGES") | Grant privileges |
| `revoke_access_to_database` | POST | `user`, `database` | Revoke all privileges |
| `list_routines` | GET | `database_user` | List stored routines |
| `get_restrictions` | GET | — | Get MySQL restrictions |
| `get_server_information` | GET | — | Get MySQL server info (version, host, port) |
| `check_database` | POST | `name` | Check/repair database |
| `repair_database` | POST | `name` | Repair database |
| `rename_database` | POST | `oldname`, `newname` | Rename database |

### Example: Create database and user with full privileges

```
cpanel(action="uapi", module="Mysql", function="create_database", method="POST",
       params={"name": "mysite_wp"})

cpanel(action="uapi", module="Mysql", function="create_user", method="POST",
       params={"name": "mysite_user", "password": "Str0ngP@ss!"})

cpanel(action="uapi", module="Mysql", function="set_privileges_on_database", method="POST",
       params={"user": "mysite_user", "database": "mysite_wp", "privileges": "ALL PRIVILEGES"})
```

Note: cPanel auto-prefixes database and user names with the account username (e.g. `cpuser_mysite_wp`).

## PostgreSQL

Module: `Postgresql`

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `list_databases` | GET | — | List PostgreSQL databases |
| `create_database` | POST | `name` | Create database |
| `delete_database` | POST | `name` | Delete database |
| `list_users` | GET | — | List users |
| `create_user` | POST | `name`, `password` | Create user |
| `delete_user` | POST | `name` | Delete user |
| `set_privileges_on_database` | POST | `user`, `database`, `privileges` | Grant privileges |

## phpMyAdmin

Module: `Session` (used for SSO login to phpMyAdmin)

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `create_webmail_session_for_mail_user` | POST | `login`, `domain` | Create session URL |
