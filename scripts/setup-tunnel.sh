#!/bin/bash
# =============================================================================
# Kiropi - Cloudflare Tunnel Setup Script
# =============================================================================
# This script sets up a Cloudflare Tunnel to expose Kiropi to the internet
# so that Kiro (MCP client) can connect to it remotely.
#
# Two modes:
#   1. Quick (temporary) - No account needed, random URL
#   2. Named (persistent) - Requires Cloudflare account, custom subdomain
# =============================================================================

set -e

KIROPI_PORT="${KIROPI_PORT:-8080}"
TUNNEL_NAME="${TUNNEL_NAME:-kiropi}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

print_banner() {
    echo -e "${BLUE}"
    echo "╦╔═╦╦═╗╔═╗╔═╗╦"
    echo "╠╩╗║╠╦╝║ ║╠═╝║"
    echo "╩ ╩╩╩╚═╚═╝╩  ╩"
    echo "Cloudflare Tunnel Setup"
    echo -e "${NC}"
}

check_cloudflared() {
    if ! command -v cloudflared &> /dev/null; then
        echo -e "${YELLOW}cloudflared not found. Installing...${NC}"
        install_cloudflared
    else
        echo -e "${GREEN}✓ cloudflared found: $(cloudflared --version)${NC}"
    fi
}

install_cloudflared() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    case $ARCH in
        x86_64) ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        armv7l) ARCH="arm" ;;
        *) echo -e "${RED}Unsupported architecture: $ARCH${NC}"; exit 1 ;;
    esac

    case $OS in
        linux)
            URL="https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-${ARCH}"
            curl -fsSL "$URL" -o /usr/local/bin/cloudflared
            chmod +x /usr/local/bin/cloudflared
            ;;
        darwin)
            if command -v brew &> /dev/null; then
                brew install cloudflared
            else
                URL="https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-darwin-${ARCH}.tgz"
                curl -fsSL "$URL" | tar xz -C /usr/local/bin/
            fi
            ;;
        *)
            echo -e "${RED}Unsupported OS: $OS${NC}"
            echo "Please install cloudflared manually: https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/"
            exit 1
            ;;
    esac

    echo -e "${GREEN}✓ cloudflared installed successfully${NC}"
}

# Quick tunnel (no account needed)
quick_tunnel() {
    echo -e "${YELLOW}Starting quick tunnel (temporary URL)...${NC}"
    echo -e "${YELLOW}Note: URL will change every time you restart${NC}"
    echo ""
    echo -e "Kiropi must be running on port ${GREEN}${KIROPI_PORT}${NC}"
    echo ""
    cloudflared tunnel --url "http://localhost:${KIROPI_PORT}"
}

# Named tunnel (persistent, requires login)
named_tunnel() {
    echo -e "${YELLOW}Setting up named tunnel: ${TUNNEL_NAME}${NC}"
    echo ""

    # Check if logged in
    if ! cloudflared tunnel list &> /dev/null 2>&1; then
        echo -e "${YELLOW}Please login to Cloudflare first:${NC}"
        cloudflared tunnel login
    fi

    # Create tunnel if it doesn't exist
    if ! cloudflared tunnel info "$TUNNEL_NAME" &> /dev/null 2>&1; then
        echo -e "${YELLOW}Creating tunnel: ${TUNNEL_NAME}${NC}"
        cloudflared tunnel create "$TUNNEL_NAME"
    else
        echo -e "${GREEN}✓ Tunnel '${TUNNEL_NAME}' already exists${NC}"
    fi

    # Get tunnel ID
    TUNNEL_ID=$(cloudflared tunnel info "$TUNNEL_NAME" 2>/dev/null | grep -oP 'ID:\s+\K[a-f0-9-]+' || true)

    if [ -z "$TUNNEL_ID" ]; then
        echo -e "${RED}Failed to get tunnel ID${NC}"
        exit 1
    fi

    echo -e "${GREEN}Tunnel ID: ${TUNNEL_ID}${NC}"
    echo ""
    echo -e "${YELLOW}Next steps:${NC}"
    echo "1. Add a DNS route (replace with your domain):"
    echo -e "   ${BLUE}cloudflared tunnel route dns ${TUNNEL_NAME} kiropi.yourdomain.com${NC}"
    echo ""
    echo "2. Create config file at ~/.cloudflared/config.yml:"
    echo -e "${BLUE}"
    cat << EOF
   tunnel: ${TUNNEL_ID}
   credentials-file: /root/.cloudflared/${TUNNEL_ID}.json

   ingress:
     - hostname: kiropi.yourdomain.com
       service: http://localhost:${KIROPI_PORT}
     - service: http_status:404
EOF
    echo -e "${NC}"
    echo "3. Run the tunnel:"
    echo -e "   ${BLUE}cloudflared tunnel run ${TUNNEL_NAME}${NC}"
    echo ""
    echo "4. Or run as a system service:"
    echo -e "   ${BLUE}cloudflared service install${NC}"
}

# Systemd service setup
setup_service() {
    echo -e "${YELLOW}Setting up Kiropi + Cloudflared as systemd services...${NC}"

    # Kiropi service
    cat > /tmp/kiropi.service << 'EOF'
[Unit]
Description=Kiropi MCP Bridge Server
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/kiropi
EnvironmentFile=/etc/kiropi/.env
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

    echo -e "${GREEN}✓ Kiropi service file created at /tmp/kiropi.service${NC}"
    echo ""
    echo "To install:"
    echo -e "  ${BLUE}sudo cp /tmp/kiropi.service /etc/systemd/system/${NC}"
    echo -e "  ${BLUE}sudo mkdir -p /etc/kiropi && sudo cp .env /etc/kiropi/.env${NC}"
    echo -e "  ${BLUE}sudo systemctl daemon-reload${NC}"
    echo -e "  ${BLUE}sudo systemctl enable --now kiropi${NC}"
}

# Main menu
print_banner

echo "Choose setup mode:"
echo ""
echo -e "  ${GREEN}1)${NC} Quick tunnel  - Temporary URL, no account needed"
echo -e "  ${GREEN}2)${NC} Named tunnel  - Persistent URL, requires Cloudflare account"
echo -e "  ${GREEN}3)${NC} Systemd setup - Install as system service"
echo -e "  ${GREEN}4)${NC} Install cloudflared only"
echo ""
read -p "Select [1-4]: " choice

case $choice in
    1)
        check_cloudflared
        quick_tunnel
        ;;
    2)
        check_cloudflared
        named_tunnel
        ;;
    3)
        setup_service
        ;;
    4)
        install_cloudflared
        ;;
    *)
        echo -e "${RED}Invalid option${NC}"
        exit 1
        ;;
esac
