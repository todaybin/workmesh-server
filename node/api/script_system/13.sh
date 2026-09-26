#!/usr/bin/env bash

# Install and configure a firewall backend supported by the detected system.

set -Eeuo pipefail
umask 077

OS=""
OS_LIKE=""
VERSION=""
VERSION_MAJOR="0"
OS_FAMILY=""
FIREWALL=""
RECOMMENDED_FIREWALL="iptables"
IPTABLES_DEPRECATED=false
TCP_INPUT=""
UDP_INPUT=""
STATE_DIR="${INSTALL_STATE_DIR:-/var/lib/1panel/firewall}"
declare -a AVAILABLE_FIREWALLS=()
declare -a TCP_PORTS=()
declare -a UDP_PORTS=()
TCP_PORT_COUNT=0
UDP_PORT_COUNT=0
COLOR_ENABLED=false

init_output() {
    if [[ -z "${NO_COLOR:-}" && ( "${FORCE_COLOR:-0}" == 1 || ( -t 1 && "${TERM:-}" != dumb ) ) ]]; then
        COLOR_ENABLED=true
    fi
}

log() {
    local level="$1"; shift
    local color=""
    if [[ "$COLOR_ENABLED" == true ]]; then
        case "$level" in
            INFO) color=$'\033[0;32m' ;;
            SUCCESS) color=$'\033[1;32m' ;;
            WARN) color=$'\033[0;33m' ;;
            ERROR) color=$'\033[0;31m' ;;
            DEBUG) color=$'\033[0;36m' ;;
        esac
    fi
    local reset=""
    [[ -n "$color" ]] && reset=$'\033[0m'
    printf '%b[%s]%b %s\n' "$color" "$level" "$reset" "$*"
}

on_error() {
    local rc=$?
    trap - ERR
    log ERROR "command failed: rc=$rc line=$1 command=$2"
    log ERROR "configuration failed"
    exit "$rc"
}

run() {
    log INFO "running: $(printf '%q ' "$@")"
    "$@"
}

usage() {
    cat <<EOF
Usage: $0 [options]

Options:
  -h, --help            Show this help

This installer is interactive. It shows only the firewall backends supported
by the detected system, then prompts for TCP and UDP ports.
EOF
}

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --help|-h)
                usage
                exit 0
                ;;
            *)
                log ERROR "unknown option: $1"
                usage
                exit 1
                ;;
        esac
    done
}

detect_os() {
    [[ $EUID -eq 0 ]] || { log ERROR "this installer must run as root"; exit 1; }
    [[ -r /etc/os-release ]] || { log ERROR "/etc/os-release is missing"; exit 1; }
    # shellcheck disable=SC1091
    . /etc/os-release
    OS="${ID:-}"; OS_LIKE="${ID_LIKE:-}"; VERSION="${VERSION_ID:-unknown}"
    VERSION_MAJOR="${VERSION%%.*}"
    [[ "$VERSION_MAJOR" =~ ^[0-9]+$ ]] || VERSION_MAJOR=0
    case "$OS" in
        ubuntu|debian|linuxmint|pop)
            OS_FAMILY="debian"
            ;;
        rhel|centos|rocky|almalinux|fedora|ol|amzn)
            OS_FAMILY="rhel"
            ;;
        *)
            if [[ " $OS_LIKE " == *" debian "* ]]; then
                OS_FAMILY="debian"
            elif [[ " $OS_LIKE " == *" rhel "* || " $OS_LIKE " == *" fedora "* ]]; then
                OS_FAMILY="rhel"
            else
                log ERROR "unsupported system: os=$OS id_like=${OS_LIKE:-none}"
                exit 1
            fi
            ;;
    esac
    if [[ "$OS_FAMILY" == debian ]]; then
        AVAILABLE_FIREWALLS=(iptables nftables ufw)
    elif (( VERSION_MAJOR >= 9 )); then
        AVAILABLE_FIREWALLS=(nftables firewalld iptables)
        IPTABLES_DEPRECATED=true
    else
        AVAILABLE_FIREWALLS=(iptables nftables firewalld)
    fi
    RECOMMENDED_FIREWALL="${AVAILABLE_FIREWALLS[0]}"
    log INFO "detected os=$OS version=$VERSION family=$OS_FAMILY recommended_firewall=$RECOMMENDED_FIREWALL"
}

select_firewall() {
    local choice=""
    local index
    [[ -t 0 ]] || { log ERROR "this installer requires an interactive terminal"; exit 1; }
    printf '\nAvailable firewall backends for %s %s:\n' "$OS" "$VERSION"
    for index in "${!AVAILABLE_FIREWALLS[@]}"; do
        if (( index == 0 )); then
            printf '  %d) %s (recommended)\n' "$((index + 1))" "${AVAILABLE_FIREWALLS[$index]}"
        elif [[ "${AVAILABLE_FIREWALLS[$index]}" == iptables && "$IPTABLES_DEPRECATED" == true ]]; then
            printf '  %d) %s (deprecated on this system)\n' "$((index + 1))" "${AVAILABLE_FIREWALLS[$index]}"
        else
            printf '  %d) %s\n' "$((index + 1))" "${AVAILABLE_FIREWALLS[$index]}"
        fi
    done
    read -r -p "Select a firewall [1]: " choice
    choice="${choice:-1}"
    if [[ ! "$choice" =~ ^[0-9]+$ ]] || (( choice < 1 || choice > ${#AVAILABLE_FIREWALLS[@]} )); then
        log ERROR "invalid firewall selection: $choice"
        exit 1
    fi
    FIREWALL="${AVAILABLE_FIREWALLS[$((choice - 1))]}"
    log INFO "selected firewall=$FIREWALL available=${AVAILABLE_FIREWALLS[*]}"
}

append_unique() {
    local value="$1"
    local array_name="$2"
    local index
    case "$array_name" in
        TCP_PORTS)
            for ((index = 0; index < TCP_PORT_COUNT; index++)); do
                if [[ "${TCP_PORTS[$index]}" == "$value" ]]; then
                    return 0
                fi
            done
            TCP_PORTS+=("$value")
            ((TCP_PORT_COUNT += 1))
            ;;
        UDP_PORTS)
            for ((index = 0; index < UDP_PORT_COUNT; index++)); do
                if [[ "${UDP_PORTS[$index]}" == "$value" ]]; then
                    return 0
                fi
            done
            UDP_PORTS+=("$value")
            ((UDP_PORT_COUNT += 1))
            ;;
        *)
            log ERROR "unknown port array: $array_name"
            exit 1
            ;;
    esac
}

validate_and_add_ports() {
    local protocol="$1"
    local input="$2"
    local token start end
    for token in $input; do
        if [[ "$token" =~ ^([0-9]+)-([0-9]+)$ ]]; then
            start="${BASH_REMATCH[1]}"; end="${BASH_REMATCH[2]}"
            if (( start < 1 || end > 65535 || start > end )); then
                log ERROR "invalid $protocol port range: $token"
                exit 1
            fi
        elif [[ "$token" =~ ^[0-9]+$ ]]; then
            if (( token < 1 || token > 65535 )); then
                log ERROR "invalid $protocol port: $token"
                exit 1
            fi
        else
            log ERROR "invalid $protocol port token: $token"
            exit 1
        fi
        if [[ "$protocol" == tcp ]]; then
            append_unique "$token" TCP_PORTS
        else
            append_unique "$token" UDP_PORTS
        fi
    done
}

collect_ports() {
    [[ -t 0 ]] || { log ERROR "this installer requires an interactive terminal"; exit 1; }
    read -r -p "TCP ports to allow (optional, for example: 22 80 443 39000-40000): " TCP_INPUT
    read -r -p "UDP ports to allow (press Enter to skip): " UDP_INPUT
    validate_and_add_ports tcp "$TCP_INPUT"
    validate_and_add_ports udp "$UDP_INPUT"
    local ssh_port=""
    if command -v sshd >/dev/null 2>&1; then
        ssh_port="$(sshd -T 2>/dev/null | awk '$1 == "port" && !found { print $2; found=1 }')"
    fi
    if [[ -n "$ssh_port" ]]; then
        append_unique "$ssh_port" TCP_PORTS
        log INFO "ensuring SSH port $ssh_port remains allowed"
    else
        log WARN "unable to determine the SSH daemon port; verify remote access before enabling the firewall"
    fi
    if (( TCP_PORT_COUNT == 0 && UDP_PORT_COUNT == 0 )); then
        log ERROR "no valid firewall ports were provided"
        exit 1
    fi
    log INFO "requested tcp_ports=${TCP_PORTS[*]:-none} udp_ports=${UDP_PORTS[*]:-none}"
}

install_firewall() {
    local pkg=""
    if [[ "$OS_FAMILY" == debian ]]; then
        export DEBIAN_FRONTEND=noninteractive
        case "$FIREWALL" in
            iptables)
                command -v iptables >/dev/null 2>&1 && command -v netfilter-persistent >/dev/null 2>&1 && return
                run apt-get update
                run apt-get install -y --no-install-recommends iptables iptables-persistent
                ;;
            nftables)
                command -v nft >/dev/null 2>&1 && return
                run apt-get update
                run apt-get install -y --no-install-recommends nftables
                ;;
            ufw)
                command -v ufw >/dev/null 2>&1 && return
                run apt-get update
                run apt-get install -y --no-install-recommends ufw
                ;;
        esac
        return
    fi
    pkg="$(command -v dnf || command -v yum || true)"
    [[ -n "$pkg" ]] || { log ERROR "dnf/yum is unavailable"; exit 1; }
    case "$FIREWALL" in
        iptables)
            if (( VERSION_MAJOR >= 9 )) || [[ "$OS" == fedora ]]; then
                run "$pkg" install -y iptables-nft iptables-nft-services
            else
                run "$pkg" install -y iptables-services
            fi
            ;;
        nftables) run "$pkg" install -y nftables ;;
        firewalld) run "$pkg" install -y firewalld ;;
    esac
}

configure_iptables() {
    command -v iptables >/dev/null 2>&1 || { log ERROR "iptables command is unavailable after installation"; exit 1; }
    iptables -P INPUT DROP || true
    iptables -A INPUT -i lo -j ACCEPT || true
    iptables -A INPUT -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT || true
    local port
    for port in "${TCP_PORTS[@]+"${TCP_PORTS[@]}"}"; do
        iptables -A INPUT -p tcp -m tcp --dport "${port/-/:}" -j ACCEPT
    done
    for port in "${UDP_PORTS[@]+"${UDP_PORTS[@]}"}"; do
        iptables -A INPUT -p udp -m udp --dport "${port/-/:}" -j ACCEPT
    done
    if [[ "$OS_FAMILY" == debian ]]; then
        run netfilter-persistent save || true
        run systemctl enable netfilter-persistent.service || true
    fi
}

configure_nftables() {
    command -v nft >/dev/null 2>&1 || { log ERROR "nft command is unavailable"; exit 1; }
    nft add table inet onepanel || true
    nft add chain inet onepanel input '{ type filter hook input priority -10; policy drop; }' || true
    nft add rule inet onepanel input iifname lo accept
    nft add rule inet onepanel input ct state established,related accept
    local port
    for port in "${TCP_PORTS[@]+"${TCP_PORTS[@]}"}"; do
        nft add rule inet onepanel input tcp dport "$port" accept
    done
    for port in "${UDP_PORTS[@]+"${UDP_PORTS[@]}"}"; do
        nft add rule inet onepanel input udp dport "$port" accept
    done
    run systemctl enable nftables.service || true
}

configure_ufw() {
    local port
    for port in "${TCP_PORTS[@]+"${TCP_PORTS[@]}"}"; do
        run ufw allow "${port/-/:}/tcp"
    done
    for port in "${UDP_PORTS[@]+"${UDP_PORTS[@]}"}"; do
        run ufw allow "${port/-/:}/udp"
    done
    run ufw --force enable
}

configure_firewalld() {
    run systemctl enable firewalld.service
    run systemctl start firewalld.service
    local port
    for port in "${TCP_PORTS[@]+"${TCP_PORTS[@]}"}"; do
        run firewall-cmd --permanent --add-port="$port/tcp"
    done
    for port in "${UDP_PORTS[@]+"${UDP_PORTS[@]}"}"; do
        run firewall-cmd --permanent --add-port="$port/udp"
    done
    run firewall-cmd --reload
}

main() {
    init_output
    trap 'on_error "$LINENO" "$BASH_COMMAND"' ERR
    log INFO "starting firewall installer"
    parse_args "$@"
    detect_os
    select_firewall
    collect_ports
    install_firewall
    case "$FIREWALL" in
        iptables) configure_iptables ;;
        nftables) configure_nftables ;;
        ufw) configure_ufw ;;
        firewalld) configure_firewalld ;;
    esac
    log SUCCESS "$FIREWALL configuration completed successfully"
}

if [[ -z "${BASH_SOURCE[0]:-}" || "${BASH_SOURCE[0]:-}" == "$0" ]]; then
    main "$@"
fi
