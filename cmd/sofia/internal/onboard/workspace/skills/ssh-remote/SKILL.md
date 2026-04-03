---
name: ssh-remote
description: "SSH into remote servers, run commands, copy files with scp/rsync, and manage remote environments."
metadata: {"nanobot":{"emoji":"🔌","os":["darwin","linux"],"requires":{"bins":["ssh","scp"]}}}
---

# SSH Remote Skill

Use this skill when asked to connect to a remote server, run commands on a remote machine, copy files to/from a server, or manage remote deployments.

## Trigger phrases

- "ssh into", "connect to server", "run on remote", "copy file to server", "deploy to", "sync to remote"

## Running remote commands

Always use single-quoted remote commands to avoid local shell expansion:

```bash
# Run a single command
ssh user@host 'ls -la /var/www'

# Run multiple commands
ssh user@host 'cd /app && git pull && systemctl restart myservice'

# Run as a different user (sudo)
ssh user@host 'sudo systemctl status nginx'
```

## Key-based authentication

Prefer key files over passwords. Never pass passwords as command-line arguments.

```bash
# Specify a key file
ssh -i ~/.ssh/id_rsa user@host 'uname -a'

# Use a non-default port
ssh -p 2222 user@host 'whoami'

# Combine key file and port
ssh -i ~/.ssh/deploy_key -p 2222 user@host 'ls /app'
```

## SSH config file

For frequently used hosts, recommend the user add entries to `~/.ssh/config`:

```
Host myserver
    HostName 192.168.1.100
    User deploy
    IdentityFile ~/.ssh/deploy_key
    Port 22
```

Then simply: `ssh myserver 'command'`

## Copying files (scp)

```bash
# Copy local file to remote
scp /local/path/file.tar.gz user@host:/remote/path/

# Copy remote file to local
scp user@host:/remote/path/file.log /local/downloads/

# Copy directory recursively
scp -r /local/dir user@host:/remote/dir

# With a key file
scp -i ~/.ssh/id_rsa /local/file user@host:/remote/
```

## Syncing files (rsync)

rsync is preferred over scp for directories — it only transfers changed files:

```bash
# Sync local dir to remote (preserves permissions, timestamps)
rsync -avz /local/dir/ user@host:/remote/dir/

# Exclude patterns
rsync -avz --exclude='*.log' --exclude='node_modules/' /local/app/ user@host:/app/

# Dry run first
rsync -avzn /local/dir/ user@host:/remote/dir/

# With a key file
rsync -avz -e "ssh -i ~/.ssh/deploy_key" /local/ user@host:/remote/
```

## Handling known_hosts

For automation environments where the host key is not yet trusted:

```bash
# Auto-accept host key (use carefully — only in trusted environments)
ssh -o StrictHostKeyChecking=no user@host 'command'

# Disable known_hosts check entirely (ephemeral environments)
ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null user@host 'command'
```

## Running scripts on remote

```bash
# Pipe a local script to remote bash
ssh user@host 'bash -s' < /local/deploy.sh

# Heredoc inline script
ssh user@host << 'EOF'
  cd /app
  git pull origin main
  npm install --production
  pm2 restart app
EOF
```

## Checking connectivity / debugging

```bash
# Test connectivity (verbose)
ssh -v user@host 'exit'

# Test with timeout (useful in scripts)
ssh -o ConnectTimeout=5 user@host 'echo ok'
```

## Safety notes

- Never pass passwords as inline arguments — use SSH agent (`ssh-add`) or key files
- When syncing with rsync, always verify the source and destination paths. A trailing `/` matters
- Use `rsync -n` (dry run) before destructive syncs
- Prefer `-o StrictHostKeyChecking=accept-new` over `=no` when possible — it accepts new hosts but rejects changed keys
