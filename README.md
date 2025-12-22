# Theia - Privacy-Friendly Page View Tracker

## Overview

Server-side analytics tool that tracks page views by parsing nginx access logs. No client-side JavaScript required, making it resistant to ad-blockers.

## Installation

### Quick Install (Recommended)

Install Theia with a single command:

```bash
# Using curl
curl -sSL https://raw.githubusercontent.com/Elysium-Labs-EU/theia/main/install.sh | sudo bash

# Or using wget
wget -qO- https://raw.githubusercontent.com/Elysium-Labs-EU/theia/main/install.sh | sudo bash
```

This will:

1. Detect and use curl or wget (whichever is available)
2. Download the latest release for your architecture (amd64/arm64)
3. Install the binary to `/usr/local/bin/theia`
4. Create a systemd service
5. Set up the data directory at `/var/lib/theia`

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

### Basic Command

```bash
# Start tracking (requires root/sudo for log access)
sudo ./theia

# Database created at ./pageviews.db
```

### Configuration

Edit `main.go` to change the log file path:

```go
tailLog("/var/log/nginx/access.log", pageViews)
```

Default assumes standard nginx access log location. Adjust based on your nginx configuration.

### Query Your Data

```bash
# View all page views
sudo sqlite3 /var/lib/theia/pageviews.db "SELECT * FROM pageviews;"

# Count views by path
sudo sqlite3 /var/lib/theia/pageviews.db "SELECT path, COUNT(*) FROM pageviews GROUP BY path;"

# Views in last hour
sudo sqlite3 /var/lib/theia/pageviews.db "SELECT * FROM pageviews WHERE timestamp > datetime('now', '-1 hour');"
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
2. Parses each log line to extract: path, referrer, user-agent, IP
3. Hashes IP addresses (SHA256) for privacy
4. Writes to SQLite database asynchronously

## Database Schema

```sql
CREATE TABLE pageviews (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME,
    path TEXT,
    referrer TEXT,
    user_agent TEXT,
    ip_hash TEXT
);
```

## Requirements

- Linux with systemd
- Go 1.21+ (for building from source)
- nginx with standard access log format
- Root/sudo access (for nginx log access)

## Limitations

- Only tracks page views (no client-side events)
- Data loss possible during crashes or restarts
- Requires standard nginx log format
- No built-in dashboard (query SQLite directly)
