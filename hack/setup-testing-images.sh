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
EOF

docker image save localhost:50000/buildah/stable:v1.31.0 | \
  docker exec -i func-control-plane ctr -n=k8s.io images import -
docker rmi localhost:50000/buildah/stable:v1.31.0
