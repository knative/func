#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

KO_DOCKER_REPO="ttl.sh/$(head -c 128 </dev/urandom | LC_CTYPE=C tr -dc 'a-z0-9' | fold -w 8 | head -n 1)"
export KO_DOCKER_REPO

REF_FILE=$(mktemp)

ko build --image-refs "${REF_FILE}" --tags "30m" -B ./cmd/func

yq -Y -i ".spec.steps[0].image = \"$(cat "${REF_FILE}")\"" \
  pipelines/resources/tekton/task/func-deploy/0.1/func-deploy.yaml
