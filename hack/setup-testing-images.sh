#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

FUNC_UTILS_IMG="localhost:50000/knative/func-utils:latest"

docker build . -f Dockerfile.utils -t "${FUNC_UTILS_IMG}"
docker push "${FUNC_UTILS_IMG}"

# Build custom buildah image for tests.
# This image will accept registries ending with .cluster.local as insecure (non-TLS).
go install github.com/google/go-containerregistry/cmd/crane@latest
crane append --base=quay.io/buildah/stable:v1.31.0 \
             --new_layer="$(dirname "$0")/allow-insecure.tar" \
             --new_tag=quay.io/buildah/stable:v1.31.0 \
             --output=/dev/stdout | \
  docker exec -i func-control-plane ctr -n=k8s.io images import -
