#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

# Push the locally built func-util image under the reference the func binary
# embeds (see docs/func-utils-image.md), with the local registry in place of
# the registry host. The kind cluster mirrors ghcr.io and quay.io to the local
# registry (hack/cluster.sh), so in-cluster pulls of that reference find it.
EMBEDDED_IMG="$(make -C "$(dirname "$0")/.." -s --no-print-directory func-utils-image)"
FUNC_UTILS_IMG="registry.localtest.me/${EMBEDDED_IMG#*/}"

CGO_ENABLED=0 go build -o "func-util" -trimpath -ldflags '-w -s' ./cmd/func-util

docker build . -f Dockerfile.utils -t "${FUNC_UTILS_IMG}" --build-arg FUNC_UTIL_BINARY=func-util
docker push "${FUNC_UTILS_IMG}"

# Build custom buildah image for tests.
# This image will accept registries ending with .cluster.local as insecure (non-TLS).
go install github.com/google/go-containerregistry/cmd/crane@v0.21.5
crane append --base=quay.io/buildah/stable:v1.31.0 \
             --new_layer="$(dirname "$0")/allow-insecure.tar" \
             --new_tag=quay.io/buildah/stable:v1.31.0 \
             --output=/dev/stdout | \
  docker exec -i func-control-plane ctr -n=k8s.io images import -
