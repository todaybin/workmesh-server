#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
# Copyright (c) 2026 WorkMesh contributors

set -Eeuo pipefail

# is_ipv4 validates the decimal form used by DNS A records and curl --resolve.
is_ipv4() {
  local value=$1
  local part
  local count=0
  IFS=. read -r -a parts <<<"$value"
  [[ "${#parts[@]}" -eq 4 ]] || return 1
  for part in "${parts[@]}"; do
    [[ "$part" =~ ^[0-9]{1,3}$ ]] || return 1
    ((10#$part <= 255)) || return 1
    ((count += 1))
  done
  [[ "$count" -eq 4 && "$value" != 127.* && "$value" != 0.* ]]
}

append_candidate() {
  local value=$1
  is_ipv4 "$value" || return 0
  for existing in "${CANDIDATES[@]}"; do
    [[ "$existing" != "$value" ]] || return 0
  done
  CANDIDATES+=("$value")
}

detect_host_ipv4() {
  local value
  local -a CANDIDATES=()

  if command -v ip >/dev/null 2>&1; then
    while read -r value; do
      append_candidate "$value"
    done < <(ip -4 -o addr show scope global 2>/dev/null | awk '{split($4, item, "/"); print item[1]}')
  fi

  if command -v hostname >/dev/null 2>&1; then
    while read -r value; do
      append_candidate "$value"
    done < <(hostname -I 2>/dev/null | tr ' ' '\n')
  fi

  # Static Debian/Ubuntu installations can still expose the address when
  # netlink is unavailable in a restricted diagnostic environment.
  if [[ -f /etc/network/interfaces ]]; then
    while read -r value; do
      append_candidate "$value"
    done < <(awk '$1 == "address" {print $2}' /etc/network/interfaces)
  fi

  if ((${#CANDIDATES[@]} > 0)); then
    printf '%s\n' "${CANDIDATES[0]}"
    return 0
  fi
  return 1
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  detect_host_ipv4 || {
    printf 'unable to detect a non-loopback IPv4 address\n' >&2
    exit 1
  }
fi
