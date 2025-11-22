#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

export FUNC_UTILS_IMG="localhost:50000/knative/func-utils:v2"
echo "[DEBUG] Exporting FUNC_UTILS_IMG = ${FUNC_UTILS_IMG}"
if [ -n "${GITHUB_ENV:-}" ]; then
    echo "[DEBUG] Found github env! copying to GITHUB_ENV variable for subsequent steps"
    echo "FUNC_UTILS_IMG=${FUNC_UTILS_IMG}" >> "$GITHUB_ENV"
fi

CGO_ENABLED=0 go build -o "func-util" -trimpath -ldflags '-w -s' ./cmd/func-util

docker build . -f Dockerfile.utils -t "${FUNC_UTILS_IMG}" --build-arg FUNC_UTIL_BINARY=func-util
docker push "${FUNC_UTILS_IMG}"
echo "Image 'docker pushed' to local registry"

# Build custom buildah image for tests.
# This image will accept registries ending with .cluster.local as insecure (non-TLS).
go install github.com/google/go-containerregistry/cmd/crane@latest
crane append --base=quay.io/buildah/stable:v1.31.0 \
             --new_layer="$(dirname "$0")/allow-insecure.tar" \
             --new_tag=quay.io/buildah/stable:v1.31.0 \
             --output=/dev/stdout | \
  docker exec -i func-control-plane ctr -n=k8s.io images import -
