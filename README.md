<p align="center">
  <img src=".github/logo.svg" alt="theia logo" width="120" height="120">
</p>

# theia - Privacy-First Server-Side Analytics

[![GitHub](https://img.shields.io/badge/GitHub-theia-blue?logo=github)](https://github.com/Elysium-Labs-EU/theia)

GitHub is the canonical repository. The Codeberg copy is a read-only mirror. Please open issues and PRs on GitHub.

## Overview

Server-side analytics tool that tracks page views by parsing nginx access logs. No client-side JavaScript required, making it resistant to ad-blockers.

## Installation

### Quick Installation

Using curl
```bash
curl -sSL https://raw.githubusercontent.com/Elysium-Labs-EU/theia/main/install.sh -o install.sh
sudo bash install.sh
```

Using wget
```bash
wget https://raw.githubusercontent.com/Elysium-Labs-EU/theia/main/install.sh
sudo bash install.sh
```

This will:

1. Detect and use curl or wget (whichever is available)
2. Download the latest release for your architecture (amd64/arm64)
3. Install the binary to `/usr/local/bin/theia`
4. Add custom nginx log formatting for multidomain tracking - if you select so
5. Create a systemd service
6. Set up the data directory at `/var/lib/theia`

Then start the service:

```bash
sudo systemctl start theia
sudo systemctl status theia
```

### Manual Installation

If you prefer to build from source:

```bash
# Clone and build
git clone https://github.com/Elysium-Labs-EU/theia.git
cd theia
go build -o theia

# Install manually
sudo cp theia /usr/local/bin/
sudo mkdir -p /var/lib/theia
# Copy theia.service to /etc/systemd/system/
# Enable and start service
```

## Usage

### Starting the daemon

```bash
# Start tracking (requires root/sudo for nginx log access)
sudo theia daemon --log-path /var/log/nginx/access.log --db-path /var/lib/theia/theia.db
```

Flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--log-path` | `/var/log/nginx/access.log` | Path to nginx access log |
| `--db-path` | `./theia.db` | Path to SQLite database |

### Querying analytics

```bash
# Last 7 days summary (default)
theia stats --db-path /var/lib/theia/theia.db

# Last 30 days for a specific host
theia stats --db-path /var/lib/theia/theia.db --days 30 --host example.com

# Machine-readable JSON output
theia stats --db-path /var/lib/theia/theia.db --format json

# Show top 20 paths instead of 10
theia stats --db-path /var/lib/theia/theia.db --top 20
```

Flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--db-path` | `./theia.db` | Path to SQLite database |
| `--days` | `7` | Number of days to look back |
| `--host` | (all hosts) | Filter by hostname |
| `--format` | `table` | Output format: `table` or `json` |
| `--top` | `10` | Number of top paths/referrers to show |

Example output:

```
Summary (last 7 days)
  Pageviews:        12345
  Unique visitors:  1023
  Bot views:        342

Top Paths
  PATH      HOST         PAGEVIEWS
  /         example.com  5000
  /about    example.com  2100

Status Codes
  CODE  COUNT
  200   11800
  404   300

Top Referrers
  REFERRER              COUNT
  https://example.com   420
```

### Shell tab completion

```bash
# Detect your shell and prompt to install
theia completion

# Or print the script for a specific shell to stdout
theia completion bash > /etc/bash_completion.d/theia
theia completion zsh > "${fpath[1]}/_theia"
theia completion fish > ~/.config/fish/completions/theia.fish
```

## Service Management

Theia runs as a systemd service:

```bash
# Start the service
sudo systemctl start theia

# Stop the service
sudo systemctl stop theia

# Check status
sudo systemctl status theia

# View logs
sudo journalctl -u theia -f

# Restart service
sudo systemctl restart theia
```

## How It Works

1. Reads nginx access logs in real-time using `tail -f`
2. Parses each log line to extract: path, referrer, user-agent, IP, status code, bytes sent
3. Hashes IP addresses with user-agent and date (SHA256) for privacy
4. Detects bots and static assets automatically
5. Writes to SQLite database asynchronously
6. Automatically cleans up old records every 12 hours:
   - Hourly stats, status codes, referrers, and visitor days: older than 60 days

## Requirements

- Linux with systemd
- Go 1.25+ (for building from source)
- Root/sudo access (for nginx log access)

## Security

See [docs/nginx-hardening.md](docs/nginx-hardening.md) for guidance on rate-limiting or banning
scanners that repeatedly probe for `wp-*.php`, `.git/config`, `sftp-config.json`, and similar
paths — these show up as 404 noise in nginx logs (and therefore in theia's stats) and are worth
blocking at the nginx/fail2ban level.

## Limitations

- Only tracks page views (no client-side events)
- Data loss possible during crashes or restarts
- No web dashboard - use `theia stats` or query SQLite directly

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.