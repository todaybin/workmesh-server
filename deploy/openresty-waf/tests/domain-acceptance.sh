#!/usr/bin/env sh
set -eu

# Read-only acceptance checks for a deployed website behind WorkMesh WAF.
# The script never changes DNS, certificates, containers, or WAF settings.

DOMAIN=${DOMAIN:-workmesh.cs.sopvip.com}
ADDRESS=${ADDRESS:-61.184.12.165}
HTTP_PORT=${HTTP_PORT:-80}
HTTPS_PORT=${HTTPS_PORT:-443}
HTTP_PATH=${HTTP_PATH:-/}
HTTPS_PATH=${HTTPS_PATH:-/}
ATTACK_PATH=${ATTACK_PATH:-/?id=1%20union%20select%201}
EXPECTED_HTTP_STATUS=${EXPECTED_HTTP_STATUS:-200,301,302,307,308}
EXPECTED_HTTPS_STATUS=${EXPECTED_HTTPS_STATUS:-200}
EXPECTED_BLOCK_STATUS=${EXPECTED_BLOCK_STATUS:-403}
TLS_VERIFY=${TLS_VERIFY:-0}
CHECK_CONTAINER=${CHECK_CONTAINER:-0}
CONTAINER_NAME=${CONTAINER_NAME:-workmesh-openresty-waf}
CHECK_LOGS=${CHECK_LOGS:-0}
WAF_LOG_DIR=${WAF_LOG_DIR:-}

die() {
    echo "ERROR: $*" >&2
    exit 2
}

require_command() {
    command -v "$1" >/dev/null 2>&1 || die "missing command: $1"
}

contains_status() {
    wanted=",$1,"
    case "$wanted" in
        *,"$2",*) return 0 ;;
        *) return 1 ;;
    esac
}

request_status() {
    scheme="$1"
    port="$2"
    path="$3"
    if [ "$scheme" = "https" ] && [ "$TLS_VERIFY" != "1" ]; then
        curl -k -sS --resolve "${DOMAIN}:${port}:${ADDRESS}" \
            -o /dev/null -w '%{http_code}' "${scheme}://${DOMAIN}:${port}${path}"
    else
        curl -sS --resolve "${DOMAIN}:${port}:${ADDRESS}" \
            -o /dev/null -w '%{http_code}' "${scheme}://${DOMAIN}:${port}${path}"
    fi
}

check_request() {
    name="$1"
    scheme="$2"
    port="$3"
    path="$4"
    expected="$5"
    actual=$(request_status "$scheme" "$port" "$path")
    if ! contains_status "$expected" "$actual"; then
        echo "FAIL ${name}: expected one of ${expected}, got ${actual}" >&2
        return 1
    fi
    echo "PASS ${name}: ${actual}"
}

[ -n "$DOMAIN" ] || die "set DOMAIN, for example DOMAIN=example.com"
[ -n "$ADDRESS" ] || die "set ADDRESS to the target IPv4/IPv6 address"
require_command curl

case "$TLS_VERIFY" in
    0|1) ;;
    *) die "TLS_VERIFY must be 0 or 1" ;;
esac

case "$CHECK_CONTAINER" in
    0) ;;
    1)
        require_command docker
        config_status=$(docker exec "$CONTAINER_NAME" /usr/local/openresty/nginx/sbin/nginx -t 2>&1) || {
            echo "$config_status" >&2
            die "OpenResty configuration check failed"
        }
        echo "PASS container OpenResty nginx -t"
        health=$(docker inspect --format '{{.State.Health.Status}}' "$CONTAINER_NAME" 2>/dev/null || true)
        [ "$health" = "healthy" ] || die "container health is ${health:-unknown}, expected healthy"
        echo "PASS container health: healthy"
        ;;
    *) die "CHECK_CONTAINER must be 0 or 1" ;;
esac

check_request "HTTP host route" http "$HTTP_PORT" "$HTTP_PATH" "$EXPECTED_HTTP_STATUS"
check_request "HTTPS host route" https "$HTTPS_PORT" "$HTTPS_PATH" "$EXPECTED_HTTPS_STATUS"
check_request "WAF attack response" https "$HTTPS_PORT" "$ATTACK_PATH" "$EXPECTED_BLOCK_STATUS"

if [ "$CHECK_LOGS" = "1" ]; then
    [ -n "$WAF_LOG_DIR" ] || die "set WAF_LOG_DIR when CHECK_LOGS=1"
    [ -d "$WAF_LOG_DIR" ] || die "WAF_LOG_DIR does not exist: $WAF_LOG_DIR"
    for log_name in access.log modsecurity-audit.json workmesh-custom-audit.jsonl; do
        [ -e "${WAF_LOG_DIR}/${log_name}" ] || die "missing WAF log: ${WAF_LOG_DIR}/${log_name}"
        echo "PASS log exists: ${log_name}"
    done
fi

echo "Domain/WAF acceptance completed for ${DOMAIN} at ${ADDRESS}"
