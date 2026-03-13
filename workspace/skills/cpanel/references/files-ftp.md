# Files & FTP Module Reference

## Fileman — File Manager

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `list_files` | GET | `dir`, `include_mime` (0/1), `include_permissions` (0/1) | List files in directory |
| `upload_files` | POST | multipart: `dir`, `file-0` | Upload file (use built-in `file_upload` action) |
| `mkdir` | POST | `dir` (parent), `name` | Create directory |
| `trash` | POST | `dir`, `files` | Move file to trash |
| `empty_trash` | POST | — | Empty the trash |
| `get_file_content` | GET | `dir`, `file` | Read file content |
| `save_file_content` | POST | `dir`, `file`, `content` | Write file content |
| `rename` | POST | `dir`, `oldname`, `newname` | Rename file/directory |
| `copy` | POST | `from`, `to`, `overwite` (sic, "1" to overwrite) | Copy file |

### Example: Read a file

```
cpanel(action="uapi", module="Fileman", function="get_file_content",
       params={"dir": "/public_html", "file": ".htaccess"})
```

### Example: Write/update a file

```
cpanel(action="uapi", module="Fileman", function="save_file_content", method="POST",
       params={"dir": "/public_html", "file": ".htaccess", "content": "RewriteEngine On\nRewriteCond %{HTTPS} off\nRewriteRule ^(.*)$ https://%{HTTP_HOST}%{REQUEST_URI} [L,R=301]"})
```

## Ftp — FTP Accounts

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `list_ftp` | GET | — | List FTP accounts |
| `list_ftp_with_disk` | GET | — | List FTP accounts with disk usage |
| `add_ftp` | POST | `user`, `pass`, `homedir`, `quota` (MB, 0=unlimited) | Create FTP account |
| `delete_ftp` | POST | `user`, `destroy` ("1" to remove files) | Delete FTP account |
| `passwd` | POST | `user`, `pass` | Change FTP password |
| `set_quota` | POST | `user`, `quota` | Change FTP quota |
| `get_port` | GET | — | Get FTP server port |

### Example: Create FTP account

```
cpanel(action="uapi", module="Ftp", function="add_ftp", method="POST",
       params={"user": "deploy", "pass": "Str0ngP@ss!", "homedir": "/public_html", "quota": "0"})
```

### Example: List FTP accounts

```
cpanel(action="uapi", module="Ftp", function="list_ftp_with_disk")
```
