#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

main() {
  local -r tmp_docker_config="$(mktemp config.json-XXXXXXXX)"

  cat <<EOF > "${tmp_docker_config}"
{
  "auths": {
    "registry.redhat.io": {
      "auth": "$(echo -n "${RH_REG_USR}:${RH_REG_PWD}" | base64 -w0)"
    }
  }
}
EOF

  local node
  for node in $(kind get nodes --name "func"); do
    tar -cf - "${tmp_docker_config}" --transform="flags=r;s|${tmp_docker_config}|config.json|" | \
      docker cp - "${node}:/var/lib/kubelet/"
  done
  rm "${tmp_docker_config}"
}

main "$@"
