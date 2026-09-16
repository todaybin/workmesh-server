#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "${SCRIPT_DIR}/.." && pwd)
IMAGE_REF=${IMAGE_REF:-}

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

case "${IMAGE_REF}" in
    "")
        fail "set IMAGE_REF to the candidate image reference"
        ;;
    *:latest|*/latest|latest)
        fail "IMAGE_REF must not use latest: ${IMAGE_REF}"
        ;;
    *:20260904|*/20260904)
        fail "IMAGE_REF still points to the legacy 20260904 image: ${IMAGE_REF}"
        ;;
    *@sha256:*)
        pass "candidate image uses an immutable digest: ${IMAGE_REF}"
        ;;
    *:*)
        pass "candidate image uses an explicit non-legacy tag: ${IMAGE_REF}"
        ;;
    *)
        fail "IMAGE_REF must include an explicit tag or digest: ${IMAGE_REF}"
        ;;
esac

assert_contains "${ROOT_DIR}/Dockerfile" "RUN /usr/local/openresty/nginx/sbin/nginx -t"
assert_contains "${ROOT_DIR}/Dockerfile" "COPY runtime/docker-entrypoint.sh /usr/local/bin/workmesh-waf-entrypoint"
assert_contains "${ROOT_DIR}/Dockerfile" "COPY runtime/modsecurity.conf /etc/workmesh-waf/modsecurity.conf"
assert_contains "${ROOT_DIR}/Dockerfile" "standard-rules.conf"
assert_contains "${ROOT_DIR}/docker-compose.yml" "./waf:/opt/workmesh/waf"
assert_contains "${ROOT_DIR}/runtime/modsecurity.conf" "IncludeOptional /opt/workmesh/waf/generated/standard-rules.conf"
assert_not_contains "${ROOT_DIR}/runtime/modsecurity.conf" "Include /etc/workmesh-waf/crs/rules/*.conf"
assert_contains "${ROOT_DIR}/tests/runtime-contract.sh" "entrypoint preserves saved generated configuration"
assert_contains "${ROOT_DIR}/build-release.sh" "docker buildx build"
assert_contains "${ROOT_DIR}/build-release.sh" "docker save"
assert_contains "${ROOT_DIR}/build-release.sh" "/usr/local/openresty/nginx/sbin/nginx -t"
pass "WAF build and runtime contracts are present"

if grep -R -E '(^|[[:space:]])(image:|-[[:space:]]+-t[[:space:]]+)[^#]*:latest([[:space:]]|$)' \
    "${ROOT_DIR}/Dockerfile" "${ROOT_DIR}/docker-compose.yml" "${ROOT_DIR}/IMAGE.md" >/dev/null 2>&1; then
    fail "WAF release files must not use latest image tags"
fi
if grep -F "20260904" "${ROOT_DIR}/docker-compose.yml" >/dev/null 2>&1; then
    fail "WAF Compose file must not default to the legacy 20260904 image"
fi
pass "release files reject latest tags and legacy Compose defaults"
