#!/usr/bin/env bash

# This script patches cluster hostname resolution for some *.127.0.0.1.sslip.io hostnames.
# pac-ctr.127.0.0.1.sslip.io => envoy.contour-external.svc.cluster.local
# gitlab.127.0.0.1.sslip.io  => gitlab-internal.gitlab.svc.cluster.local
# This ensures that these hosts are resolved to the same services as on localhost.

function patch_hosts() {
  local pac_ctr_addr="0.0.0.0"
  local gitlab_addr="0.0.0.0"

  if kubectl get svc/gitlab-internal -n gitlab > /dev/null 2>&1; then
    gitlab_addr="$(kubectl get svc/gitlab-internal -n gitlab -ojson | jq '.spec.clusterIP' -r)";
  fi

  if kubectl get svc/envoy -n contour-external > /dev/null 2>&1; then
    pac_ctr_addr="$(kubectl get svc/envoy -n contour-external -ojson | jq '.spec.clusterIP' -r)"
  fi

  kubectl patch cm/coredns -n kube-system --patch-file /dev/stdin <<EOF
{
  "data": {
    "Corefile": ".:53 {\n    errors\n    health {\n       lameduck 5s\n    }\n    ready\n    kubernetes cluster.local in-addr.arpa ip6.arpa {\n       pods insecure\n       fallthrough in-addr.arpa ip6.arpa\n       ttl 30\n    }\n    prometheus :9153\n    forward . /etc/resolv.conf {\n       max_concurrent 1000\n    }\n    cache 30\n    loop\n    reload\n    loadbalance\n    hosts /etc/coredns/customdomains.db 127.0.0.1.sslip.io {\n      ${pac_ctr_addr} pac-ctr.127.0.0.1.sslip.io\n      ${gitlab_addr} gitlab.127.0.0.1.sslip.io\n    }\n}\n"
  }
}
EOF
  kubectl rollout restart deployment coredns -n kube-system
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
