#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

KO_DOCKER_REPO="localhost:50000/knative/func"
export KO_DOCKER_REPO

ko build --tags "latest" -B ./cmd/func
