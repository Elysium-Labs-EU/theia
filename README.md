# theia - Privacy-Friendly Page View Tracker

## Overview

Server-side analytics tool that tracks page views by parsing nginx access logs. No client-side JavaScript required, making it resistant to ad-blockers.

## Installation

### Quick Install (Recommended)

Using curl
```bash
curl -sSL https://raw.githubusercontent.com/Elysium-Labs-EU/theia/main/install.sh
sudo bash install.sh
```

Using wget
```bash
wget -qO- https://raw.githubusercontent.com/Elysium-Labs-EU/theia/main/install.sh
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

### Basic Command

```bash
# Start tracking (requires root/sudo for log access)
sudo ./theia

# Database created at ./theia.db
```

### Configuration

Edit `main.go` to change the log file path:

```go
tailLog([]string{"-f", "/var/log/nginx/access.log"}, pageViews)
```

Default assumes standard nginx access log location. Adjust based on your nginx configuration.

### Query Your Data

SQLite command-line tool is automatically installed during setup. Here are some useful queries:

```bash
# View hourly stats for today (non-static traffic)
sudo sqlite3 /var/lib/theia/theia.db << 'EOF'
.mode column
.headers on
SELECT 
  hour,
  path,
  host,
  page_views,
  unique_visitors
FROM hourly_stats
WHERE year_day = CAST(strftime('%j', 'now') AS INTEGER)
  AND year = CAST(strftime('%Y', 'now') AS INTEGER)
  AND is_static = 0 
ORDER BY hour DESC, page_views DESC;
EOF

# Most popular pages in the last 24 hours
sudo sqlite3 /var/lib/theia/theia.db << 'EOF'
.mode column
.headers on
SELECT 
  path,
  host,
  SUM(page_views) as total_views,
  SUM(unique_visitors) as total_unique,
  SUM(bot_views) as total_bots
FROM hourly_stats
WHERE year_day >= CAST(strftime('%j', 'now', '-1 day') AS INTEGER)
  AND year = CAST(strftime('%Y', 'now') AS INTEGER)
  AND is_static = 0
GROUP BY path, host
ORDER BY total_views DESC
LIMIT 20;
EOF

# Traffic trends by hour of day (last 7 days)
sudo sqlite3 /var/lib/theia/theia.db << 'EOF'
.mode column
.headers on
SELECT 
  hour,
  year_day,
  year,
  date(year || '-01-01', '+' || (year_day - 1) || ' days') as actual_date,
  SUM(page_views) as total_views,
  SUM(unique_visitors) as total_unique,
  ROUND(AVG(page_views), 2) as avg_views_per_hour
FROM hourly_stats
WHERE date(year || '-01-01', '+' || (year_day - 1) || ' days') >= date('now', '-7 days')
  AND is_static = 0
GROUP BY year, year_day, hour
ORDER BY year, year_day, hour;
EOF

# Status codes distribution (last 24 hours)
sudo sqlite3 /var/lib/theia/theia.db << 'EOF'
.mode column
.headers on
SELECT 
  status_code,
  SUM(count) as total_count,
  ROUND(100.0 * SUM(count) / (SELECT SUM(count) FROM hourly_status_codes 
    WHERE year_day >= CAST(strftime('%j', 'now', '-1 day') AS INTEGER)), 2) as percentage
FROM hourly_status_codes
WHERE date(year || '-01-01', '+' || (year_day - 1) || ' days') >= date('now', '-1 day')
GROUP BY status_code
ORDER BY total_count DESC;
EOF

# 404 errors by path (last 7 days)
sudo sqlite3 /var/lib/theia/theia.db << 'EOF'
.mode column
.headers on
SELECT 
  path,
  host,
  SUM(count) as error_count
FROM hourly_status_codes
WHERE status_code = 404
  AND date(year || '-01-01', '+' || (year_day - 1) || ' days') >= date('now', '-7 days')
GROUP BY path, host
ORDER BY error_count DESC
LIMIT 20;
EOF

# Top referrers (last 7 days)
sudo sqlite3 /var/lib/theia/theia.db << 'EOF'
.mode column
.headers on
SELECT 
  referrer,
  SUM(count) as visit_count
FROM hourly_referrers
WHERE referrer != '-' AND referrer != ''
  AND date(year || '-01-01', '+' || (year_day - 1) || ' days') >= date('now', '-7 days')
GROUP BY referrer
ORDER BY visit_count DESC
LIMIT 20;
EOF

# Referrers by specific page (last 7 days) - Adjust the WHERE clause
sudo sqlite3 /var/lib/theia/theia.db << 'EOF'
.mode column
.headers on
SELECT 
  referrer,
  SUM(count) as visit_count
FROM hourly_referrers
WHERE path = '/your-page-path'
  AND referrer != '-' AND referrer != ''
  AND date(year || '-01-01', '+' || (year_day - 1) || ' days') >= date('now', '-7 days')
GROUP BY referrer
ORDER BY visit_count DESC
LIMIT 10;
EOF

# Bot vs human traffic comparison (last 7 days)
sudo sqlite3 /var/lib/theia/theia.db << 'EOF'
.mode column
.headers on
SELECT 
  SUM(page_views) as human_views,
  SUM(bot_views) as bot_views,
  SUM(unique_visitors) as total_unique_visitors,
  ROUND(100.0 * SUM(bot_views) / (SUM(page_views) + SUM(bot_views)), 2) as bot_percentage
FROM hourly_stats
WHERE date(year || '-01-01', '+' || (year_day - 1) || ' days') >= date('now', '-7 days');
EOF

# Daily traffic summary (last 30 days)
sudo sqlite3 /var/lib/theia/theia.db << 'EOF'
.mode column
.headers on
SELECT 
  year_day,
  year,
  SUM(page_views) as daily_views,
  SUM(bot_views) as daily_bots,
  COUNT(DISTINCT path) as unique_paths
FROM hourly_stats
WHERE date(year || '-01-01', '+' || (year_day - 1) || ' days') >= date('now', '-30 days')
GROUP BY year_day, year
ORDER BY year DESC, year_day DESC;
EOF

# Peak traffic hours (last 7 days)
sudo sqlite3 /var/lib/theia/theia.db << 'EOF'
.mode column
.headers on
SELECT 
  hour,
  SUM(page_views) as total_views,
  ROUND(AVG(page_views), 2) as avg_views,
  MAX(page_views) as peak_views
FROM hourly_stats
WHERE date(year || '-01-01', '+' || (year_day - 1) || ' days') >= date('now', '-7 days')
GROUP BY hour
ORDER BY total_views DESC
LIMIT 10;
EOF

# Current active unique visitors (from visitor_hashes)
sudo sqlite3 /var/lib/theia/theia.db << 'EOF'
.mode column
.headers on
SELECT 
  hour_bucket,
  COUNT(*) as unique_visitors
FROM visitor_hashes
WHERE datetime(first_seen) > datetime('now', '-1 day')
GROUP BY hour_bucket
ORDER BY hour_bucket;
EOF

# Path performance by host (last 7 days)
sudo sqlite3 /var/lib/theia/theia.db << 'EOF'
.mode column
.headers on
SELECT 
  host,
  path,
  SUM(page_views) as views,
  SUM(unique_visitors) as uniques,
  ROUND(1.0 * SUM(page_views) / SUM(unique_visitors), 2) as views_per_visitor
FROM hourly_stats
WHERE date(year || '-01-01', '+' || (year_day - 1) || ' days') >= date('now', '-7 days')
GROUP BY host, path
HAVING views > 10
ORDER BY views DESC
LIMIT 20;
EOF

# Export hourly stats to CSV (last 30 days)
sudo sqlite3 -header -csv /var/lib/theia/theia.db \
  "SELECT * FROM hourly_stats 
   WHERE date(year || '-01-01', '+' || (year_day - 1) || ' days') >= date('now', '-30 days')
   ORDER BY year DESC, year_day DESC, hour DESC;" > hourly_stats_export.csv

# Export referrer analysis to CSV
sudo sqlite3 -header -csv /var/lib/theia/theia.db \
  "SELECT referrer, path, SUM(count) as total_visits
   FROM hourly_referrers
   WHERE year_day >= CAST(strftime('%j', 'now', '-7 days') AS INTEGER)
     AND referrer != '-' AND referrer != ''
   GROUP BY referrer, path
   ORDER BY total_visits DESC;" > referrer_analysis.csv

# Database maintenance - manually trigger old data cleanup
sudo sqlite3 /var/lib/theia/theia.db << 'EOF'
-- Delete stats older than 60 days
DELETE FROM hourly_stats
WHERE year < CAST(strftime('%Y', 'now', '-60 days') AS INTEGER)
   OR (year = CAST(strftime('%Y', 'now', '-60 days') AS INTEGER) 
       AND year_day < CAST(strftime('%j', 'now', '-60 days') AS INTEGER));

DELETE FROM hourly_status_codes
WHERE year < CAST(strftime('%Y', 'now', '-60 days') AS INTEGER)
   OR (year = CAST(strftime('%Y', 'now', '-60 days') AS INTEGER) 
       AND year_day < CAST(strftime('%j', 'now', '-60 days') AS INTEGER));

DELETE FROM hourly_referrers
WHERE year < CAST(strftime('%Y', 'now', '-60 days') AS INTEGER)
   OR (year = CAST(strftime('%Y', 'now', '-60 days') AS INTEGER) 
       AND year_day < CAST(strftime('%j', 'now', '-60 days') AS INTEGER));

-- Delete old visitor hashes
DELETE FROM visitor_hashes
WHERE datetime(first_seen) < datetime('now', '-1 day');

SELECT 'Cleanup complete' as status;
EOF

# View database size and row counts
sudo sqlite3 /var/lib/theia/theia.db << 'EOF'
.mode column
.headers on
SELECT 
  'hourly_stats' as table_name,
  COUNT(*) as row_count
FROM hourly_stats
UNION ALL
SELECT 
  'hourly_status_codes',
  COUNT(*)
FROM hourly_status_codes
UNION ALL
SELECT 
  'hourly_referrers',
  COUNT(*)
FROM hourly_referrers
UNION ALL
SELECT 
  'visitor_hashes',
  COUNT(*)
FROM visitor_hashes;
EOF
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
   - Hourly stats, status codes, and referrers: older than 60 days
   - Visitor hashes: older than 1 day

## Requirements

- Linux with systemd
- Go 1.25+ (for building from source)
- Root/sudo access (for nginx log access)

## Limitations

- Only tracks page views (no client-side events)
- Data loss possible during crashes or restarts
- No built-in dashboard (query SQLite directly)
