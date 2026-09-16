#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
IMAGE_REF=${IMAGE_REF:-workmesh/openresty-waf:$(date +%Y%m%d)}
PLATFORM=${PLATFORM:-linux/amd64}
OUTPUT_DIR=${OUTPUT_DIR:-"${SCRIPT_DIR}/dist"}
DEBIAN_MIRROR=${DEBIAN_MIRROR:-deb.debian.org}
PUSH=${PUSH:-0}
SAVE=${SAVE:-1}
CONTAINER_NAME=${CONTAINER_NAME:-workmesh-waf-release-test}

fail() {
    echo "FAIL $*" >&2
    exit 1
}

info() {
    echo "INFO $*"
}

case "${IMAGE_REF}" in
    *:latest|*/latest|latest|"")
        fail "IMAGE_REF must be an explicit non-latest tag or digest target"
        ;;
    *@sha256:*)
        fail "IMAGE_REF is a build tag, not a digest; use an explicit tag and record the resulting digest"
        ;;
esac

command -v docker >/dev/null 2>&1 || fail "docker command is required"
docker version >/dev/null 2>&1 || fail "docker daemon is not available to the current user"

mkdir -p "${OUTPUT_DIR}"

IMAGE_REF="${IMAGE_REF}" sh "${SCRIPT_DIR}/tests/image-release-contract.sh"
sh "${SCRIPT_DIR}/tests/runtime-contract.sh"

build_args="--build-arg DEBIAN_MIRROR=${DEBIAN_MIRROR} --platform ${PLATFORM} -t ${IMAGE_REF} ${SCRIPT_DIR}"
if docker buildx version >/dev/null 2>&1; then
    if [ "${PUSH}" = "1" ]; then
        info "building and pushing ${IMAGE_REF} for ${PLATFORM}"
        # shellcheck disable=SC2086
        docker buildx build ${build_args} --push
    else
        case "${PLATFORM}" in
            *,*) fail "PUSH=0 with docker buildx requires one platform so the image can be loaded locally: ${PLATFORM}" ;;
        esac
        info "building ${IMAGE_REF} for local docker engine"
        # shellcheck disable=SC2086
        docker buildx build ${build_args} --load
    fi
else
    [ "${PUSH}" = "1" ] && fail "docker buildx is required when PUSH=1"
    [ "${PLATFORM}" = "linux/amd64" ] || fail "docker buildx is required for PLATFORM=${PLATFORM}"
    info "building ${IMAGE_REF} with docker build"
    docker build --build-arg "DEBIAN_MIRROR=${DEBIAN_MIRROR}" -t "${IMAGE_REF}" "${SCRIPT_DIR}"
fi

if [ "${PUSH}" != "1" ]; then
    docker run --rm "${IMAGE_REF}" /usr/local/openresty/nginx/sbin/nginx -t
    docker rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true
    docker run -d --name "${CONTAINER_NAME}" "${IMAGE_REF}" >/dev/null
    trap 'docker rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true' EXIT INT TERM
    sleep 2
    docker exec "${CONTAINER_NAME}" /usr/local/openresty/nginx/sbin/nginx -t
    docker exec "${CONTAINER_NAME}" test -f /opt/workmesh/waf/generated/standard-rules.conf
    docker exec "${CONTAINER_NAME}" grep -F "Include /etc/workmesh-waf/crs/rules/*.conf" /opt/workmesh/waf/generated/standard-rules.conf >/dev/null
    docker rm -f "${CONTAINER_NAME}" >/dev/null
    trap - EXIT INT TERM
fi

inspect_ref="${IMAGE_REF}"
if docker image inspect "${IMAGE_REF}" >/dev/null 2>&1; then
    image_id=$(docker image inspect --format '{{.Id}}' "${IMAGE_REF}")
    created=$(docker image inspect --format '{{.Created}}' "${IMAGE_REF}")
else
    image_id=$(docker buildx imagetools inspect "${IMAGE_REF}" | awk '/Digest:/ {print $2; exit}')
    [ -n "${image_id}" ] || fail "unable to read pushed image digest for ${IMAGE_REF}"
    created=""
fi

manifest="${OUTPUT_DIR}/workmesh-openresty-waf-manifest.txt"
{
    echo "image=${IMAGE_REF}"
    echo "platform=${PLATFORM}"
    echo "image_id=${image_id}"
    echo "created=${created}"
    echo "built_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
} >"${manifest}"

if [ "${SAVE}" = "1" ] && docker image inspect "${IMAGE_REF}" >/dev/null 2>&1; then
    archive="${OUTPUT_DIR}/$(printf '%s' "${IMAGE_REF}" | tr '/:' '__').tar.gz"
    docker save "${IMAGE_REF}" | gzip -c >"${archive}"
    sha256sum "${archive}" >"${archive}.sha256"
    echo "archive=${archive}" >>"${manifest}"
    echo "archive_sha256=$(cut -d ' ' -f 1 "${archive}.sha256")" >>"${manifest}"
fi

info "release manifest written: ${manifest}"
info "candidate image ready: ${inspect_ref}"
