#!/bin/bash
set -e

if [ $EUID -ne 0 ]; then
    echo "Script needs to be run as root"
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
    VERSION=$(curl -sSL "https://api.github.com/Elysium-Labs-EU/theia/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/' )
fi

curl -L -o /tmp/theia "https://github.com/Elysium-Labs-EU/theia/releases/download/$VERSION/theia-linux-$ARCH"

if [ $? -ne 0 ]; then
    echo "Download failed"
    exit 1
fi
if [ ! -f /tmp/theia ]; then
    echo "Binary not found after download"
    exit 1
fi


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