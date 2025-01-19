#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

alpine_version='3.21.2'
go_version='1.23.5'

alpine_release_key='0482D84022F52DF1C4E7CD43293ACD0907D9495A'
google_release_key='EB4C1BFD4F042F6DDDCCEC917721F63BD38B4796'

artifacts_dir="$(dirname "$(realpath "$0")")/../.artifacts"
if [ ! -d "$artifacts_dir" ]; then
  mkdir "$artifacts_dir";
fi

GNUPGHOME="$(mktemp -d)"
export GNUPGHOME
gpg --keyserver 'keyserver.ubuntu.com' --recv-keys "$alpine_release_key"
gpg --keyserver 'keyserver.ubuntu.com' --recv-keys "$google_release_key"

declare -A arch_map
arch_map['x86_64']='amd64'
arch_map['aarch64']='arm64'

function fetch_alpine_minirootfs() {
  local alpine_rel_url='https://dl-cdn.alpinelinux.org/alpine/latest-stable/releases'

  local arch
  for arch in 'x86_64' 'aarch64' 'ppc64le' 's390x'; do
    local arch_alt="${arch_map[$arch]:-$arch}"
    local tarball="${artifacts_dir}/alpine-minirootfs-${arch_alt}.tar.gz"
    local signature="${artifacts_dir}/alpine-minirootfs-${arch_alt}.tar.gz.asc"
    curl -sSL "${alpine_rel_url}/${arch}/alpine-minirootfs-${alpine_version}-${arch}.tar.gz" > \
      "${tarball}"
    curl -sSL "${alpine_rel_url}/${arch}/alpine-minirootfs-${alpine_version}-${arch}.tar.gz.asc" > \
      "${signature}"
    gpg --assert-signer="$alpine_release_key" --verify "${signature}" "${tarball}"
  done
}

function fetch_golang() {

  local arch
  arch="$(uname -m)"
  local arch_alt="${arch_map[$arch]:-$arch}"
  local tarball="${artifacts_dir}/go.tar.gz"
  local signature="${artifacts_dir}/go.tar.gz.asc"
  curl -sSL "https://go.dev/dl/go${go_version}.linux-${arch_alt}.tar.gz" > "${tarball}"
  curl -sSL "https://go.dev/dl/go${go_version}.linux-${arch_alt}.tar.gz.asc" > "${signature}"
  gpg --assert-signer="$google_release_key" --verify "${signature}" "${tarball}"
}

function main() {
  fetch_alpine_minirootfs
  fetch_golang
}

main "$@"
