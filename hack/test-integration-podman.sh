#!/usr/bin/env bash

# TCP mode: podman is running remotely (e.g. in a Fedora container).
# Just point DOCKER_HOST at it and run tests.
if [ -n "$PODMAN_NEXT_TCP" ]; then
  export DOCKER_HOST="$PODMAN_NEXT_TCP"
  make test-integration
  exit $?
fi

# Local mode: start a local podman service and configure registries.
cat <<EOF > registries.conf
unqualified-search-registries = ["docker.io", "quay.io", "registry.fedoraproject.org", "registry.access.redhat.com"]
short-name-mode="permissive"

[[registry]]
location="localhost:50000"
insecure=true
EOF

CONTAINERS_REGISTRIES_CONF="$(pwd)/registries.conf"
export CONTAINERS_REGISTRIES_CONF

podman system service --time=0 --log-level=info > podman_log.txt 2>&1 &
podman_pid=$!

DOCKER_HOST="unix://$(podman info -f '{{.Host.RemoteSocket.Path}}' 2> /dev/null)"
export DOCKER_HOST
make test-integration
e=$?

kill -TERM "$podman_pid" > /dev/null 2>&1
wait "$podman_pid" > /dev/null 2>&1

echo '::group::Podman Output'
cat podman_log.txt
echo ''
echo '::endgroup::'

exit $e
