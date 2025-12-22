# Theia - Privacy-Friendly Page View Tracker

## Overview

Server-side analytics tool that tracks page views by parsing nginx access logs. No client-side JavaScript required, making it resistant to ad-blockers.

## Installation

```bash
# Clone and build
go build -o theia

# Requires sqlite3 driver
go get github.com/mattn/go-sqlite3
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
sqlite3 pageviews.db "SELECT * FROM pageviews;"

# Count views by path
sqlite3 pageviews.db "SELECT path, COUNT(*) FROM pageviews GROUP BY path;"

# Views in last hour
sqlite3 pageviews.db "SELECT * FROM pageviews WHERE timestamp > datetime('now', '-1 hour');"
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

- Go 1.16+
- nginx with standard access log format
- Read access to nginx logs (typically requires sudo)

## Limitations

- Only tracks page views (no client-side events)
- Data loss possible during crashes or restarts
- Requires standard nginx log format
- No built-in dashboard (query SQLite directly)
