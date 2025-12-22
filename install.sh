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

echo "Downloading Theia $VERSION for linux-$ARCH..."

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

echo "Creating data directory..."
mkdir -p /var/lib/theia

echo "Installing systemd service..."
cat > /etc/systemd/system/theia.service << 'EOF'
[Unit]
Description=Theia Analytics - Privacy-Friendly Page View Tracker
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

echo "Enabling Theia service..."
systemctl daemon-reload
systemctl enable theia.service

echo ""
echo "Theia installed successfully!"
echo ""
echo "Next steps:"
echo "  1. Start the service:    sudo systemctl start theia"
echo "  2. Check status:         sudo systemctl status theia"
echo "  3. View logs:            sudo journalctl -u theia -f"
echo ""