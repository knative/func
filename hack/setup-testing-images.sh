#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

KO_DOCKER_REPO="localhost:50000/knative/func"
export KO_DOCKER_REPO

ko build --tags "latest" -B ./cmd/func

# Build custom buildah image for tests.
# This image will accept registries ending with .cluster.local as insecure (non-TLS).
docker build . -f - -t localhost:50000/buildah/stable:v1.31.0 <<EOF
FROM quay.io/buildah/stable:v1.31.0
RUN echo -e '\n[[registry]]\nprefix = "*.cluster.local"\ninsecure = true' >> '/etc/containers/registries.conf'
ENV STORAGE_DRIVER=overlay
EOF
docker push localhost:50000/buildah/stable:v1.31.0
