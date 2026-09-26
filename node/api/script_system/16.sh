#!/bin/bash
# Install and Configure Rsync
# Support Ubuntu/Debian/CentOS/RHEL/Alpine/Arch Linux

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

detect_os() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS=$ID
        VERSION=$VERSION_ID
    elif type lsb_release >/dev/null 2>&1; then
        OS=$(lsb_release -si | tr '[:upper:]' '[:lower:]')
        VERSION=$(lsb_release -sr)
    elif [ -f /etc/redhat-release ]; then
        OS="rhel"
        VERSION=$(grep -oE '[0-9]+\.[0-9]+' /etc/redhat-release)
    elif [ -f /etc/alpine-release ]; then
        OS="alpine"
        VERSION=$(cat /etc/alpine-release)
    else
        OS=$(uname -s | tr '[:upper:]' '[:lower:]')
        VERSION=$(uname -r)
    fi
}

install_rsync() {
    echo -e "${GREEN}Detected system: $OS $VERSION${NC}"

    if command -v rsync >/dev/null 2>&1; then
        echo -e "${YELLOW}Rsync is already installed: $(rsync --version | head -n1)${NC}"
        return 0
    fi

    echo -e "${BLUE}Installing rsync...${NC}"

    case "$OS" in
        ubuntu|debian)
            apt-get update
            apt-get install -y rsync
            ;;
        centos|rhel|fedora)
            if [ "$OS" = "rhel" ] && [ "${VERSION%%.*}" -ge 8 ]; then
                dnf install -y rsync
            else
                yum install -y rsync
            fi
            ;;
        alpine)
            apk add --no-cache rsync
            ;;
        arch)
            pacman -Sy --noconfirm rsync
            ;;
        *)
            echo -e "${RED}Unsupported system: $OS${NC}"
            exit 1
            ;;
    esac

    echo -e "${GREEN}Rsync installed successfully!${NC}"
}

configure_rsync() {
    echo -e "${GREEN}Configuring rsync...${NC}"

    RSYNCD_CONF="/etc/rsyncd.conf"
    RSYNCD_SECRETS="/etc/rsyncd.secrets"
    RSYNCD_MOTD="/etc/rsyncd.motd"

    if [ ! -f "$RSYNCD_CONF" ]; then
        echo -e "${BLUE}Creating basic rsyncd.conf...${NC}"
        cat <<EOF > "$RSYNCD_CONF"
# Rsync daemon configuration
uid = nobody
gid = nobody
use chroot = yes
max connections = 4
pid file = /var/run/rsyncd.pid
exclude = lost+found/
transfer logging = yes
timeout = 600
ignore nonreadable = yes
dont compress = *.gz *.tgz *.zip *.z *.Z *.rpm *.deb *.bz2
EOF
        echo -e "${GREEN}Basic rsyncd.conf created at $RSYNCD_CONF${NC}"
    else
        echo -e "${YELLOW}rsyncd.conf already exists at $RSYNCD_CONF${NC}"
    fi

    if [ ! -f "$RSYNCD_MOTD" ]; then
        cat <<EOF > "$RSYNCD_MOTD"
Welcome to this rsync server.
EOF
        echo -e "${GREEN}Created rsyncd.motd at $RSYNCD_MOTD${NC}"
    fi

    if [ ! -f "$RSYNCD_SECRETS" ]; then
        cat <<EOF > "$RSYNCD_SECRETS"
# Format: username:password
EOF
        chmod 600 "$RSYNCD_SECRETS"
        echo -e "${GREEN}Created example rsyncd.secrets at $RSYNCD_SECRETS${NC}"
    fi
}

setup_systemd_service() {
    echo -e "${BLUE}Setting up rsync daemon service...${NC}"

    if command -v systemctl >/dev/null 2>&1; then
        SYSTEMD_SERVICE="/etc/systemd/system/rsync.service"
        if [ ! -f "$SYSTEMD_SERVICE" ] && ! systemctl list-unit-files | grep -q "^rsync.service"; then
            cat <<EOF > "$SYSTEMD_SERVICE"
[Unit]
Description=Rsync daemon
After=network.target

[Service]
Type=notify
ExecStart=/usr/bin/rsync --daemon --no-detach
ExecReload=/bin/kill -HUP \$MAINPID
KillMode=process
Restart=on-failure
RestartSec=1

[Install]
WantedBy=multi-user.target
EOF
        fi
        systemctl daemon-reload
        systemctl enable rsync
        echo -e "${GREEN}Rsync daemon service enabled${NC}"
    fi
}

start_service() {
    echo -e "${GREEN}Starting rsync service...${NC}"
    if command -v systemctl >/dev/null 2>&1; then
        systemctl start rsync || true
    fi
}

check_status() {
    echo -e "${BLUE}Checking rsync status...${NC}"
    if command -v rsync >/dev/null 2>&1; then
        echo -e "${GREEN}Rsync version: $(rsync --version | head -n1)${NC}"
    else
        echo -e "${RED}Rsync not found${NC}"
        return 1
    fi
    echo -e "${GREEN}Rsync installation completed!${NC}"
}

main() {
    echo -e "${BLUE}=== Rsync Installation Script ===${NC}"
    detect_os
    install_rsync
    configure_rsync
    setup_systemd_service
    start_service
    check_status
}

main "$@"
