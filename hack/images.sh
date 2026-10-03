#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

# Push the locally built func-util image under the same tag the func binary
# embeds (FUNC_UTILS_IMG in Makefile: v2 on main/PRs, X.Y on a release-X.Y
# branch). The kind cluster mirrors ghcr.io to the local registry
# (hack/cluster.sh), so repo path and tag must match for in-cluster pulls.
EMBEDDED_IMG="$(make -C "$(dirname "$0")/.." --no-print-directory -s func-utils-image)"
FUNC_UTILS_IMG="registry.localtest.me/knative/func-utils:${EMBEDDED_IMG##*:}"

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
