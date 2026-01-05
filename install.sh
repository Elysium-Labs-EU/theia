#!/bin/bash
set -e

if [ $EUID -ne 0 ]; then
    echo "Script needs to be run as root"
    exit 1
fi


if command -v curl &> /dev/null; then
    DOWNLOAD_CMD="curl -sSL"
    DOWNLOAD_OUTPUT="-o"
elif command -v wget &> /dev/null; then
    DOWNLOAD_CMD="wget -qO"
    DOWNLOAD_OUTPUT=""
else
    echo "Error: Neither curl nor wget is installed."
    echo "Please install one of them:"
    echo "  Debian/Ubuntu: apt-get install curl"
    echo "  RHEL/CentOS:   yum install curl"
    exit 1
fi

ARCH=$(uname -m)

case $ARCH in
    x86_64)
        ARCH="amd64"
        ;;
    aarch64|arm64)
        ARCH="arm64"
        ;;
    *)
        echo "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

VERSION="${THEIA_VERSION:-}"

if [ -z "$VERSION" ]; then
    echo "Fetching latest version..."
    if command -v curl &> /dev/null; then
        VERSION=$(curl -sSL "https://api.github.com/repos/Elysium-Labs-EU/theia/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
    else
        VERSION=$(wget -qO- "https://api.github.com/repos/Elysium-Labs-EU/theia/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
    fi
    
    if [ -z "$VERSION" ]; then
        echo "Failed to fetch latest version"
        exit 1
    fi
    echo "Latest version: $VERSION"
fi

echo "Downloading theia $VERSION for linux-$ARCH..."

DOWNLOAD_URL="https://github.com/Elysium-Labs-EU/theia/releases/download/$VERSION/theia-linux-$ARCH"

if command -v curl &> /dev/null; then
    curl -L -o /tmp/theia "$DOWNLOAD_URL"
else
    wget -O /tmp/theia "$DOWNLOAD_URL"
fi

if [ $? -ne 0 ]; then
    echo "Download failed"
    echo "Tried URL: $DOWNLOAD_URL"
    exit 1
fi

if [ ! -f /tmp/theia ]; then
    echo "Binary not found after download"
    exit 1
fi

echo "Installing binary..."
chmod +x /tmp/theia
cp /tmp/theia /usr/local/bin/theia

echo "Binary installed to /usr/local/bin/theia"

echo "Installing SQLite3..."
if command -v apt-get &> /dev/null; then
    apt-get update -qq && apt-get install -y -qq sqlite3 > /dev/null 2>&1
elif command -v yum &> /dev/null; then
    yum install -y -q sqlite > /dev/null 2>&1
elif command -v apk &> /dev/null; then
    apk add --quiet sqlite > /dev/null 2>&1
else
    echo "Warning: Could not install sqlite3 automatically."
    echo "Please install it manually to query your data:"
    echo "  Debian/Ubuntu: apt-get install sqlite3"
    echo "  RHEL/CentOS:   yum install sqlite"
    echo "  Alpine:        apk add sqlite"
fi

echo "Creating data directory..."
mkdir -p /var/lib/theia

configure_nginx_multidomain() {
    echo ""
    echo "=== Nginx Multi-Domain Configuration ==="
    echo ""
    echo "If you're hosting multiple websites on this server, theia can track"
    echo "each domain separately by modifying your Nginx log format."
    echo ""
    echo "This will:"
    echo "  1. Create a custom log format that includes the hostname"
    echo "  2. Update the default access_log to use this new format"
    echo "  3. Back up your current Nginx configuration"
    echo "  4. Check for any sites with custom logging that may need manual updates"
    echo ""
    echo "After this change, all server blocks will automatically use the new format"
    echo "unless they have their own explicit access_log directives."
    echo ""
    
    # Check if Nginx is installed
    if ! command -v nginx &> /dev/null; then
        echo "Nginx is not installed. Skipping multi-domain configuration."
        return
    fi
    
    # Ask user if they want to configure multi-domain support
    read -p "Do you want to configure Nginx for multi-domain tracking? [y/N] " -n 1 -r
    echo
    
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Skipping Nginx configuration."
        return
    fi
    
    # Find nginx.conf location
    NGINX_CONF="/etc/nginx/nginx.conf"
    if [ ! -f "$NGINX_CONF" ]; then
        echo "Error: Could not find nginx.conf at $NGINX_CONF"
        echo "Please configure manually or specify the correct path."
        return
    fi
    
    # Create backup
    BACKUP_FILE="$NGINX_CONF.backup.$(date +%Y%m%d_%H%M%S)"
    echo "Creating backup: $BACKUP_FILE"
    cp "$NGINX_CONF" "$BACKUP_FILE"
    
    # Check if our custom log format already exists
    if grep -q "log_format theia_combined" "$NGINX_CONF"; then
        echo "Custom log format 'theia_combined' already exists in nginx.conf"
        echo "Skipping log format addition."
    else
        # Add custom log format to the http block
        echo "Adding custom log format to nginx.conf..."
        
        # This sed command finds the 'http {' block and adds our log format after it
        # We place it early in the http block so it's defined before being used
        sed -i '/http[[:space:]]*{/a\
\
    # Custom log format for theia analytics with hostname tracking\
    log_format theia_combined '\''$remote_addr - $remote_user [$time_local] '\''\
                              '\''"$request" $status $body_bytes_sent '\''\
                              '\''"$http_referer" "$http_user_agent" '\''\
                              '\''"$host"'\'';' "$NGINX_CONF"
        
        echo "✓ Custom log format added to nginx.conf"
    fi
    
    # Now update the default access_log directive in the http block
    # This is the key change - we're modifying the default that all server blocks inherit
    echo "Updating default access_log directive..."
    
    # Check if there's already an access_log directive in the http block
    # We need to be careful here - we only want to modify the one in the http block,
    # not ones that might be in server blocks or other nested contexts
    if grep -q "^\s*access_log.*;" "$NGINX_CONF"; then
        # Replace the existing access_log directive
        # This regex looks for access_log at the start of a line (possibly with whitespace)
        # and replaces it with our new directive
        sed -i 's|^\(\s*\)access_log\s\+[^;]*;|\1access_log /var/log/nginx/access.log theia_combined;|' "$NGINX_CONF"
        echo "✓ Updated default access_log to use theia_combined format"
    else
        # If there's no access_log directive, add one after the log_format
        # This ensures it comes after our custom format definition
        sed -i '/log_format theia_combined/,/;/a\
\
    # Default access log using theia format\
    access_log /var/log/nginx/access.log theia_combined;' "$NGINX_CONF"
        echo "✓ Added default access_log directive with theia_combined format"
    fi
    
    echo ""
    echo "=== Checking for Sites with Custom Logging ==="
    echo ""
    
    # Check all sites in sites-available and sites-enabled for custom access_log directives
    # These would override our default and need manual attention
    SITES_WITH_OVERRIDES=()
    
    # Check sites-available
    if [ -d "/etc/nginx/sites-available" ]; then
        while IFS= read -r site_file; do
            # Look for access_log directives in this site configuration
            # We grep for lines that contain access_log but aren't commented out
            if grep -q "^\s*access_log" "$site_file"; then
                # Extract just the filename without the path for cleaner display
                site_name=$(basename "$site_file")
                SITES_WITH_OVERRIDES+=("$site_name")
            fi
        done < <(find /etc/nginx/sites-available -type f)
    fi
    
    # Report findings to the user
    if [ ${#SITES_WITH_OVERRIDES[@]} -eq 0 ]; then
        echo "✓ No sites found with custom access_log directives."
        echo "All your sites will automatically use the new theia_combined format!"
    else
        echo "⚠ Found ${#SITES_WITH_OVERRIDES[@]} site(s) with custom access_log directives:"
        echo ""
        
        for site in "${SITES_WITH_OVERRIDES[@]}"; do
            echo "  - $site"
        done
        
        echo ""
        echo "These sites have their own access_log directives that will override"
        echo "the default we just set. This means theia won't be able to track"
        echo "visits to these domains unless you manually update them."
        echo ""
        echo "To fix this, you need to modify each of these files and change their"
        echo "access_log line to use the theia_combined format. For example:"
        echo ""
        echo "  Before:"
        echo "    access_log /var/log/nginx/example.log combined;"
        echo ""
        echo "  After:"
        echo "    access_log /var/log/nginx/example.log theia_combined;"
        echo ""
        echo "Or, if you want all sites to use the default logging, you can simply"
        echo "remove the access_log directive from these files entirely, and they'll"
        echo "inherit the default we just configured."
        echo ""
    fi
    
    echo "=== Configuration Summary ==="
    echo ""
    echo "✓ Custom log format 'theia_combined' is defined in nginx.conf"
    echo "✓ Default access_log now uses theia_combined format"
    echo "✓ All server blocks without explicit access_log directives will automatically"
    echo "  include the hostname in their logs, enabling multi-domain tracking"
    echo ""

    # Ask if they want to test the configuration now
    read -p "Do you want to test the Nginx configuration now? [y/N] " -n 1 -r
    echo
    
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo "Testing Nginx configuration..."
        if nginx -t 2>&1; then
            echo "✓ Nginx configuration is valid!"
            echo ""
            read -p "Do you want to reload Nginx now? [y/N] " -n 1 -r
            echo
            if [[ $REPLY =~ ^[Yy]$ ]]; then
                if systemctl reload nginx 2>&1; then
                    echo "✓ Nginx reloaded successfully"
                    echo ""
                    echo "Multi-domain tracking is now active! Nginx is logging with hostnames."
                else
                    echo "✗ Failed to reload Nginx"
                    echo "Please check the service status: sudo systemctl status nginx"
                fi
            fi
        else
            echo "✗ Nginx configuration test failed!"
            echo "Restoring backup..."
            cp "$BACKUP_FILE" "$NGINX_CONF"
            echo "Backup restored. Please check your configuration manually."
            echo "The backup file is still available at: $BACKUP_FILE"
        fi
    else
        echo ""
        echo "Remember to test and reload Nginx when you're ready:"
        echo "  sudo nginx -t"
        echo "  sudo systemctl reload nginx"
    fi
}

configure_nginx_multidomain

echo "Installing systemd service..."
cat > /etc/systemd/system/theia.service << 'EOF'
[Unit]
Description=theia Analytics - Privacy-Friendly Page View Tracker
After=network.target nginx.service

[Service]
Type=simple
User=root
WorkingDirectory=/var/lib/theia
ExecStart=/usr/local/bin/theia
Restart=always
RestartSec=10

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/theia
ReadOnlyPaths=/var/log/nginx

[Install]
WantedBy=multi-user.target
EOF

echo "Enabling theia service..."
systemctl daemon-reload
systemctl enable theia.service

echo ""
echo "theia installed successfully!"
echo ""
echo "Next steps:"
echo "  1. Start the service:    sudo systemctl start theia"
echo "  2. Check status:         sudo systemctl status theia"
echo "  3. View logs:            sudo journalctl -u theia -f"
echo ""