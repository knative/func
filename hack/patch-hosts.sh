#!/usr/bin/env bash

# This script creates a DNS A records for '127.0.0.1.sslip.io' and '*.127.0.0.1.sslip.io' pointing to the cluster node.

function patch_hosts() {
  local cluster_node_addr

  cluster_node_addr="$(docker container inspect func-control-plane | jq ".[0].NetworkSettings.Networks.kind.IPAddress" -r)"

  kubectl patch cm/coredns -n kube-system --patch-file /dev/stdin <<EOF
{
  "data": {
    "Corefile": ".:53 {\n    errors\n    health {\n       lameduck 5s\n    }\n    ready\n    kubernetes cluster.local in-addr.arpa ip6.arpa {\n       pods insecure\n       fallthrough in-addr.arpa ip6.arpa\n       ttl 30\n    }\n    file /etc/coredns/example.db 127.0.0.1.sslip.io\n    prometheus :9153\n    forward . /etc/resolv.conf {\n       max_concurrent 1000\n    }\n    cache 30\n    loop\n    reload\n    loadbalance\n}\n",
    "example.db": "; 127.0.0.1.sslip.io test file\n127.0.0.1.sslip.io.            IN      SOA     sns.dns.icann.org. noc.dns.icann.org. 2015082541 7200 3600 1209600 3600\n127.0.0.1.sslip.io.            IN      A       ${cluster_node_addr}\n*.127.0.0.1.sslip.io.          IN      A       ${cluster_node_addr}\n"
  }
}
EOF

  kubectl patch deploy/coredns -n kube-system --patch-file /dev/stdin <<EOF
{
  "spec": {
    "template": {
      "spec": {
        "\$setElementOrder/volumes": [
          {
            "name": "config-volume"
          }
        ],
        "volumes": [
          {
            "\$retainKeys": [
              "configMap",
              "name"
            ],
            "configMap": {
              "items": [
                {
                  "key": "Corefile",
                  "path": "Corefile"
                },
                {
                  "key": "example.db",
                  "path": "example.db"
                }
              ]
            },
            "name": "config-volume"
          }
        ]
      }
    }
  }
}
EOF
  sleep 1
  kubectl wait pod --for=condition=Ready -l '!job-name' -n kube-system --timeout=15s
}

if [ "$0" = "${BASH_SOURCE[0]}" ]; then
  set -o errexit
  set -o nounset
  set -o pipefail

  function main() {
    patch_hosts
  }
  main "$@"
fi
