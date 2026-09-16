#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "${SCRIPT_DIR}/.." && pwd)

fail() {
    echo "FAIL $*" >&2
    exit 1
}

pass() {
    echo "PASS $*"
}

assert_contains() {
    file="$1"
    needle="$2"
    grep -Fq "$needle" "$file" || fail "${file} missing: ${needle}"
}

assert_not_contains() {
    file="$1"
    needle="$2"
    if grep -Fq "$needle" "$file"; then
        fail "${file} must not contain: ${needle}"
    fi
}

assert_contains "${ROOT_DIR}/docker-compose.yml" "./waf:/opt/workmesh/waf"
assert_contains "${ROOT_DIR}/docker-compose.yml" "healthcheck:"
assert_contains "${ROOT_DIR}/docker-compose.yml" "/usr/local/openresty/nginx/sbin/nginx"
assert_contains "${ROOT_DIR}/Dockerfile" "COPY runtime/docker-entrypoint.sh /usr/local/bin/workmesh-waf-entrypoint"
assert_contains "${ROOT_DIR}/Dockerfile" "ENTRYPOINT [\"/usr/local/bin/workmesh-waf-entrypoint\"]"
assert_contains "${ROOT_DIR}/Dockerfile" "CMD [\"/usr/local/openresty/nginx/sbin/nginx\", \"-g\", \"daemon off;\"]"
assert_contains "${ROOT_DIR}/Dockerfile" "Include /etc/workmesh-waf/crs/crs-setup.conf"
assert_contains "${ROOT_DIR}/Dockerfile" "Include /etc/workmesh-waf/crs/rules/*.conf"
pass "image startup contract is declared"

assert_contains "${ROOT_DIR}/runtime/nginx.conf" "modsecurity on;"
assert_contains "${ROOT_DIR}/runtime/nginx.conf" "modsecurity_rules_file /etc/workmesh-waf/modsecurity.conf;"
assert_contains "${ROOT_DIR}/runtime/modsecurity.conf" "IncludeOptional /opt/workmesh/waf/generated/modsecurity-mode.conf"
assert_contains "${ROOT_DIR}/runtime/modsecurity.conf" "IncludeOptional /opt/workmesh/waf/generated/custom-rules.conf"
assert_contains "${ROOT_DIR}/runtime/modsecurity.conf" "IncludeOptional /opt/workmesh/waf/generated/standard-rules.conf"
assert_contains "${ROOT_DIR}/runtime/modsecurity.conf" "IncludeOptional /opt/workmesh/waf/generated/global-rules.conf"
assert_not_contains "${ROOT_DIR}/runtime/modsecurity.conf" "Include /etc/workmesh-waf/crs/crs-setup.conf"
assert_not_contains "${ROOT_DIR}/runtime/modsecurity.conf" "Include /etc/workmesh-waf/crs/rules/*.conf"
pass "OpenResty loads generated WorkMesh ModSecurity files"

tmp="${TMPDIR:-/tmp}/workmesh-waf-contract.$$"
trap 'rm -rf "$tmp"' EXIT INT TERM
mkdir -p "$tmp"

WORKMESH_WAF_ROOT="${tmp}/waf" sh "${ROOT_DIR}/runtime/docker-entrypoint.sh" true
assert_contains "${tmp}/waf/generated/modsecurity-mode.conf" "SecRuleEngine DetectionOnly"
assert_contains "${tmp}/waf/generated/standard-rules.conf" "Include /etc/workmesh-waf/crs/crs-setup.conf"
assert_contains "${tmp}/waf/generated/standard-rules.conf" "Include /etc/workmesh-waf/crs/rules/*.conf"
[ -d "${tmp}/waf/logs" ] || fail "entrypoint did not create logs directory"
pass "entrypoint initializes empty bind-mounted WAF root"

printf '%s\n' "# saved by control plane" >"${tmp}/waf/generated/standard-rules.conf"
WORKMESH_WAF_ROOT="${tmp}/waf" sh "${ROOT_DIR}/runtime/docker-entrypoint.sh" true
assert_contains "${tmp}/waf/generated/standard-rules.conf" "# saved by control plane"
assert_not_contains "${tmp}/waf/generated/standard-rules.conf" "Include /etc/workmesh-waf/crs/crs-setup.conf"
pass "entrypoint preserves saved generated configuration"
