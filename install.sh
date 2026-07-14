#!/bin/bash
set -euo pipefail

readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly CYAN='\033[0;36m'
readonly BOLD='\033[1m'
readonly DIM='\033[2m'
readonly NC='\033[0m'

readonly REPO="Elysium_Labs/theia"
readonly CODEBERG_URL="https://codeberg.org"
readonly BINARY_NAME="theia"
readonly INSTALL_DIR="${THEIA_INSTALL_DIR:-/usr/local/bin}"
readonly DATA_DIR="/var/lib/theia"

AUTO_YES=false

info() {
    echo -e "${BLUE}${BOLD}info${NC} $1"
}

success() {
    echo -e "${GREEN}${BOLD}✓${NC} $1"
}

warn() {
    echo -e "${YELLOW}${BOLD}warning${NC} $1"
}

error() {
    echo -e "${RED}${BOLD}error${NC} $1" >&2
}

step() {
    echo -e "\n${CYAN}${BOLD}→${NC} $1"
}

dim() {
    echo -e "${DIM}$1${NC}"
}

usage() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  --local <path>    Use a local binary instead of downloading from Codeberg"
    echo "  --help            Show this help message"
    echo "  --yes, -y         Skip all confirmation prompts (non-interactive mode)"
    echo ""
    echo "Environment variables:"
    echo "  THEIA_INSTALL_DIR   Install directory (default: /usr/local/bin)"
    echo "  THEIA_VERSION       Version to install (default: latest)"
}

confirm() {
    local prompt="$1"
    local default="${2:-n}"

    if [ "$AUTO_YES" = true ]; then
        [[ "$default" =~ ^[Yy]$ ]]
        return $?
    fi

    local response

    if [ "$default" = "y" ]; then
        prompt="$prompt [Y/n]"
    else
        prompt="$prompt [y/N]"
    fi

    echo -ne "${YELLOW}?${NC} $prompt "
    read -r response

    response=${response:-$default}
    [[ "$response" =~ ^[Yy]$ ]]
}

check_root() {
    if [ $EUID -ne 0 ]; then
        error "This script must be run as root"
        dim "  Try: sudo $0"
        exit 1
    fi
}

detect_download_tool() {
    if command -v curl &> /dev/null; then
        echo "curl"
    elif command -v wget &> /dev/null; then
        echo "wget"
    else
        error "Neither curl nor wget is installed"
        echo ""
        echo "Please install one of them:"
        dim "  Debian/Ubuntu: apt-get install curl"
        dim "  RHEL/CentOS:   yum install curl"
        dim "  Alpine:        apk add curl"
        exit 1
    fi
}

download_file() {
    local url="$1"
    local output="$2"
    local tool="$3"

    if [ "$tool" = "curl" ]; then
        curl -fsSL -o "$output" "$url" 2>&1 | sed 's/^/  /'
    else
        wget -q --show-progress -O "$output" "$url" 2>&1 | sed 's/^/  /'
    fi
}

fetch_json_field() {
    local url="$1"
    local field="$2"
    local tool="$3"

    local response
    if [ "$tool" = "curl" ]; then
        response=$(curl -fsSL "$url")
    else
        response=$(wget -qO- "$url")
    fi

    echo "$response" | grep -o "\"$field\":\"[^\"]*\"" | sed -E 's/"[^"]+":"([^"]+)"/\1/' | head -1
}

detect_arch() {
    local arch
    arch=$(uname -m)

    case $arch in
        x86_64)
            echo "amd64"
            ;;
        aarch64|arm64)
            echo "arm64"
            ;;
        *)
            error "Unsupported architecture: $arch"
            dim "  Supported: x86_64, aarch64/arm64"
            exit 1
            ;;
    esac
}

detect_package_manager() {
    if command -v apt-get &> /dev/null; then
        echo "apt"
    elif command -v dnf &> /dev/null; then
        echo "dnf"
    elif command -v yum &> /dev/null; then
        echo "yum"
    elif command -v apk &> /dev/null; then
        echo "apk"
    elif command -v pacman &> /dev/null; then
        echo "pacman"
    else
        echo "unknown"
    fi
}

# sqlite3 CLI is optional — theia embeds its own SQLite. Install it so users
# can query /var/lib/theia/theia.db directly if they want.
install_sqlite3_cli() {
    local pkg_manager="$1"

    step "Checking sqlite3 CLI..."

    if command -v sqlite3 &> /dev/null; then
        local version
        version=$(sqlite3 --version | cut -d' ' -f1)
        success "sqlite3 already installed (${version})"
        return 0
    fi

    info "sqlite3 CLI not found"
    dim "  Not required to run theia, but useful for querying ${DATA_DIR}/theia.db"
    echo ""

    if [ "$pkg_manager" = "unknown" ]; then
        warn "Unknown package manager — skipping sqlite3 install"
        return 0
    fi

    if ! confirm "Install sqlite3 CLI now?" "y"; then
        warn "Skipping sqlite3 install"
        return 0
    fi

    case $pkg_manager in
        apt)  apt-get update -qq && apt-get install -y -qq sqlite3 > /dev/null 2>&1 ;;
        yum)  yum install -y -q sqlite > /dev/null 2>&1 ;;
        dnf)  dnf install -y -q sqlite > /dev/null 2>&1 ;;
        apk)  apk add --quiet sqlite > /dev/null 2>&1 ;;
        pacman) pacman -S --noconfirm --quiet sqlite > /dev/null 2>&1 ;;
    esac

    if command -v sqlite3 &> /dev/null; then
        success "sqlite3 installed"
    else
        warn "sqlite3 install may have failed — check manually"
    fi
}

stop_running_service() {
    if ! systemctl is-active --quiet theia.service 2>/dev/null; then
        return 0
    fi

    echo ""
    warn "theia.service is running"
    dim "  Replacing binary while the service is active can fail (Text file busy)"
    echo ""

    if confirm "Stop theia service before installing?" "y"; then
        systemctl stop theia.service
        success "Service stopped"
        WAS_RUNNING=true
    else
        warn "Continuing with service running — install may fail"
    fi
}

configure_nginx_multidomain() {
    echo ""
    step "Nginx multi-domain configuration"
    echo ""
    info "theia can track each domain separately by adding the hostname to your Nginx log format."
    dim "  This modifies nginx.conf to add a custom log_format and updates the default access_log."
    echo ""

    if ! command -v nginx &> /dev/null; then
        info "Nginx not installed — skipping"
        return
    fi

    if ! confirm "Configure Nginx for multi-domain tracking?" "y"; then
        info "Skipping Nginx configuration"
        return
    fi

    local nginx_conf="/etc/nginx/nginx.conf"
    if [ ! -f "$nginx_conf" ]; then
        error "Could not find nginx.conf at ${nginx_conf}"
        warn "Configure manually if your path differs"
        return
    fi

    local backup_file="${nginx_conf}.backup.$(date +%Y%m%d_%H%M%S)"
    cp "$nginx_conf" "$backup_file"
    success "Backup created: ${backup_file}"

    if grep -q "log_format theia_combined" "$nginx_conf"; then
        info "log_format 'theia_combined' already exists — skipping addition"
    else
        sed -i '/http[[:space:]]*{/a\
\
    # Custom log format for theia analytics with hostname tracking\
    log_format theia_combined '\''$remote_addr - $remote_user [$time_local] '\''\
                              '\''"$request" $status $body_bytes_sent '\''\
                              '\''"$http_referer" "$http_user_agent" '\''\
                              '\''"$host"'\'';' "$nginx_conf"
        success "Added log_format theia_combined to nginx.conf"
    fi

    if grep -q "^\s*access_log.*;" "$nginx_conf"; then
        sed -i 's|^\(\s*\)access_log\s\+[^;]*;|\1access_log /var/log/nginx/access.log theia_combined;|' "$nginx_conf"
        success "Updated default access_log to use theia_combined format"
    else
        sed -i '/log_format theia_combined/,/;/a\
\
    access_log /var/log/nginx/access.log theia_combined;' "$nginx_conf"
        success "Added default access_log with theia_combined format"
    fi

    echo ""
    info "Checking for sites with custom access_log directives..."

    local sites_with_overrides=()
    if [ -d "/etc/nginx/sites-available" ]; then
        while IFS= read -r site_file; do
            if grep -q "^\s*access_log" "$site_file"; then
                sites_with_overrides+=("$(basename "$site_file")")
            fi
        done < <(find /etc/nginx/sites-available -type f)
    fi

    if [ ${#sites_with_overrides[@]} -eq 0 ]; then
        success "No sites with custom access_log — all will use theia_combined automatically"
    else
        warn "Found ${#sites_with_overrides[@]} site(s) with custom access_log directives:"
        for site in "${sites_with_overrides[@]}"; do
            dim "  - $site"
        done
        echo ""
        dim "  These override the default. Update them manually to use theia_combined:"
        dim "    access_log /var/log/nginx/example.log theia_combined;"
    fi

    echo ""
    if confirm "Test Nginx configuration now?" "y"; then
        if nginx -t 2>&1; then
            success "Nginx configuration valid"
            echo ""
            if confirm "Reload Nginx now?" "y"; then
                if systemctl reload nginx 2>&1; then
                    success "Nginx reloaded — multi-domain tracking active"
                else
                    error "Failed to reload Nginx"
                    dim "  Check: sudo systemctl status nginx"
                fi
            fi
        else
            error "Nginx configuration test failed — restoring backup"
            cp "$backup_file" "$nginx_conf"
            success "Backup restored"
            dim "  Backup still at: ${backup_file}"
        fi
    else
        dim "  Test and reload when ready:"
        dim "    sudo nginx -t && sudo systemctl reload nginx"
    fi
}

install_systemd_service() {
    step "Installing systemd service..."

    cat > /etc/systemd/system/theia.service << EOF
[Unit]
Description=theia - Privacy-First Server-Side Analytics
After=network.target nginx.service

[Service]
Type=simple
User=root
WorkingDirectory=${DATA_DIR}
ExecStart=${INSTALL_DIR}/${BINARY_NAME} daemon
Restart=always
RestartSec=10

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${DATA_DIR}
ReadOnlyPaths=/var/log/nginx

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable theia.service
    success "Service installed and enabled"
}

refresh_completions() {
    local theia_bin="${INSTALL_DIR}/${BINARY_NAME}"
    local target_user="${SUDO_USER:-$(whoami)}"
    local target_home
    target_home=$(getent passwd "$target_user" 2>/dev/null | cut -d: -f6)

    if [ -z "$target_home" ]; then
        return 0
    fi

    # Keep in sync with completionTargetPath() in cmd/completion.go
    local bash_completion="${target_home}/.local/share/bash-completion/completions/${BINARY_NAME}"
    local zsh_completion="${target_home}/.zsh/completions/_${BINARY_NAME}"
    local fish_completion="${target_home}/.config/fish/completions/${BINARY_NAME}.fish"

    local refreshed=false
    if [ -f "$bash_completion" ] && "$theia_bin" completion bash > "$bash_completion" 2>/dev/null; then
        refreshed=true
    fi
    if [ -f "$zsh_completion" ] && "$theia_bin" completion zsh > "$zsh_completion" 2>/dev/null; then
        refreshed=true
    fi
    if [ -f "$fish_completion" ] && "$theia_bin" completion fish > "$fish_completion" 2>/dev/null; then
        refreshed=true
    fi

    if [ "$refreshed" = true ]; then
        success "Refreshed shell completion for ${target_user}"
    fi
}

main() {
    local local_binary=""
    WAS_RUNNING=false

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --local)
                if [[ $# -lt 2 ]]; then
                    error "--local requires a path argument"
                    usage
                    exit 1
                fi
                local_binary="$2"
                shift 2
                ;;
            --local=*)
                local_binary="${1#*=}"
                shift
                ;;
            --help|-h)
                usage
                exit 0
                ;;
            --yes|-y)
                AUTO_YES=true
                shift
                ;;
            *)
                error "Unknown option: $1"
                usage
                exit 1
                ;;
        esac
    done

    if [ -n "$local_binary" ] && [ ! -f "$local_binary" ]; then
        error "Local binary not found: $local_binary"
        exit 1
    fi

    echo ""
    echo -e "${BOLD}theia installer${NC}"
    echo ""

    info "Running pre-flight checks..."
    check_root

    local download_tool
    download_tool=$(detect_download_tool)
    dim "  Download tool: $download_tool"

    local arch
    arch=$(detect_arch)
    dim "  Architecture: $arch"

    local pkg_manager
    pkg_manager=$(detect_package_manager)
    dim "  Package manager: $pkg_manager"

    if [ "$INSTALL_DIR" != "/usr/local/bin" ]; then
        dim "  Install directory: $INSTALL_DIR (custom)"
    fi

    echo ""

    local version=""
    if [ -z "$local_binary" ]; then
        version="${THEIA_VERSION:-}"
        if [ -z "$version" ]; then
            step "Fetching latest version..."
            version=$(fetch_json_field "${CODEBERG_URL}/api/v1/repos/${REPO}/releases?limit=1" "tag_name" "$download_tool")

            if [ -z "$version" ]; then
                error "Failed to fetch latest version"
                dim "  Set THEIA_VERSION environment variable to specify manually"
                exit 1
            fi

            info "Latest version: ${BOLD}$version${NC}"
        else
            info "Using version: ${BOLD}$version${NC}"
        fi
    else
        info "Using local binary: ${BOLD}$local_binary${NC}"
    fi

    echo ""
    echo -e "${BOLD}Installation plan:${NC}"
    if [ -n "$local_binary" ]; then
        echo "  1. Use local binary: ${local_binary}"
    else
        echo "  1. Download binary from Codeberg"
    fi
    echo "  2. Install to ${INSTALL_DIR}/${BINARY_NAME}"
    echo "  3. Install sqlite3 CLI (optional, for querying ${DATA_DIR}/theia.db)"
    echo "  4. Configure Nginx for multi-domain tracking (optional)"
    echo "  5. Install and enable systemd service"
    echo "  6. Create data directory at ${DATA_DIR}"
    echo ""

    if ! confirm "Continue with installation?" "y"; then
        info "Installation cancelled"
        exit 0
    fi

    # Download or use local binary
    local tmp_binary
    if [ -n "$local_binary" ]; then
        tmp_binary="$local_binary"
        success "Using local binary"
    else
        echo ""
        step "Downloading ${BINARY_NAME} ${version} for linux-${arch}..."

        local download_url="${CODEBERG_URL}/${REPO}/releases/download/${version}/theia-linux-${arch}"
        tmp_binary="/tmp/${BINARY_NAME}"

        if ! download_file "$download_url" "$tmp_binary" "$download_tool"; then
            error "Download failed"
            dim "  URL: $download_url"
            exit 1
        fi

        if [ ! -f "$tmp_binary" ]; then
            error "Binary not found after download"
            exit 1
        fi

        success "Downloaded successfully"
    fi

    # Stop running service before overwriting binary
    stop_running_service

    # Install binary
    step "Installing binary..."
    mkdir -p "$INSTALL_DIR"
    chmod +x "$tmp_binary"
    local final_binary="${INSTALL_DIR}/${BINARY_NAME}"
    local tmp_install="${final_binary}.tmp.$$"
    cp "$tmp_binary" "$tmp_install"
    mv -f "$tmp_install" "$final_binary"
    success "Installed to ${final_binary}"

    # Refresh any shell completion already installed for the invoking user
    refresh_completions

    # Optional sqlite3 CLI
    install_sqlite3_cli "$pkg_manager"

    # Optional nginx config
    configure_nginx_multidomain

    # Systemd service
    install_systemd_service

    # Data directory
    step "Creating data directory..."
    mkdir -p "$DATA_DIR"
    success "Created ${DATA_DIR}"

    if [ "$WAS_RUNNING" = true ]; then
        step "Restarting theia service..."
        systemctl start theia.service
        success "Service restarted"
    fi

    echo ""
    echo -e "${GREEN}${BOLD}Installation complete!${NC}"
    echo ""
    echo -e "${BOLD}Next steps:${NC}"
    if [ "$WAS_RUNNING" = true ]; then
        echo "  1. Check status:"
    else
        echo "  1. Start the service:"
        echo -e "     ${CYAN}sudo systemctl start theia${NC}"
        echo ""
        echo "  2. Check status:"
    fi
    echo -e "     ${CYAN}sudo systemctl status theia${NC}"
    echo ""
    echo "  3. View logs:"
    echo -e "     ${CYAN}sudo journalctl -u theia -f${NC}"
    echo ""
    echo -e "${BOLD}Enable tab completion:${NC}"
    echo -e "  bash:  ${CYAN}theia completion bash > /etc/bash_completion.d/theia${NC}"
    echo -e "  zsh:   ${CYAN}theia completion zsh > \"\${fpath[1]}/_theia\"${NC}"
    echo -e "  fish:  ${CYAN}theia completion fish > ~/.config/fish/completions/theia.fish${NC}"
    echo ""
    dim "Database: ${DATA_DIR}/theia.db"
    echo ""
}

main "$@"
