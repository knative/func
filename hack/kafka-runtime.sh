#!/usr/bin/env bash

# Copyright 2026 The Knative Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Builds the Kafka runtime (knative.dev/func-adapter-kafka) that local
# `func run` executes for functions consuming from Kafka, for one platform, into
# pkg/kafkaruntime/bin/<goos>_<goarch>/, from where it is embedded into the func
# binary built for that platform (see pkg/kafkaruntime).
#
#   hack/kafka-runtime.sh [GOOS GOARCH]   (default: the current go env)
#
# The runtime is built from the module version pinned by kafkaruntime.Version,
# with its own go.mod, so it is the same program as the image the cluster
# deployers inject. Platforms without a bin directory are skipped: their func
# build embeds no runtime. A binary already built from the pinned version is
# kept.

set -o errexit
set -o nounset
set -o pipefail

goos="${1:-$(go env GOOS)}"
goarch="${2:-$(go env GOARCH)}"

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
module="knative.dev/func-adapter-kafka/cmd/fkafka-runtime"
version="$(sed -n 's/^const Version = "\(.*\)"$/\1/p' "${root}/pkg/kafkaruntime/kafkaruntime.go")"
if [[ -z "${version}" ]]; then
  echo "no Version constant in pkg/kafkaruntime/kafkaruntime.go" >&2
  exit 1
fi

dir="${root}/pkg/kafkaruntime/bin/${goos}_${goarch}"
if [[ ! -d "${dir}" ]]; then
  echo "Kafka runtime: not embedded for ${goos}/${goarch}"
  exit 0
fi

name="fkafka-runtime"
if [[ "${goos}" == "windows" ]]; then
  name="${name}.exe"
fi
if [[ -f "${dir}/${name}" && "$(cat "${dir}/VERSION" 2>/dev/null)" == "${version}" ]]; then
  echo "Kafka runtime: ${version} for ${goos}/${goarch} is up to date"
  exit 0
fi

echo "Kafka runtime: building ${version} for ${goos}/${goarch}"
tmp="$(mktemp -d)"
trap 'rm -rf "${tmp}"' EXIT
# `go install module@version` builds with the module's own go.mod. GOBIN must
# be unset to cross-compile, so install into a throwaway GOPATH (sharing the
# module cache) and pick the binary up from there. GOFLAGS is cleared so func's
# own build flags (e.g. its -ldflags) do not apply.
modcache="$(go env GOMODCACHE)"
env -u GOBIN GOFLAGS= GOWORK=off GOPATH="${tmp}" GOMODCACHE="${modcache}" \
  CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" \
  go install -trimpath -ldflags "-w -s" "${module}@${version}"
bin="$(find "${tmp}/bin" -type f -name "${name}" | head -n 1)"
if [[ -z "${bin}" ]]; then
  echo "Kafka runtime: go install produced no ${name}" >&2
  exit 1
fi
rm -f "${dir}/VERSION"
cp "${bin}" "${dir}/${name}"
chmod 0755 "${dir}/${name}"
echo "${version}" > "${dir}/VERSION"
