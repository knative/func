#!/usr/bin/env bash

# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

#
# Sets up a WireGuard tunnel for IPv6 connectivity on CI runners.
# GitHub Actions runners lack IPv6 support; this script uses a WireGuard
# tunnel (configured via the WG_CONFIG env var) to provide IPv6 and
# DNS64+NAT64 connectivity needed for IPv6-only Kind clusters.
#
# Usage: sudo -E ./hack/wg-setup.sh
# Requires: WG_CONFIG environment variable containing wireguard config
#

set -o errexit
set -o nounset
set -o pipefail

if [[ -z "${WG_CONFIG:-}" ]]; then
  echo "WG_CONFIG not set, skipping WireGuard setup"
  exit 0
fi

echo "Installing wireguard-tools..."
apt-get update -qq
apt-get install -y -qq wireguard-tools

echo "Configuring WireGuard..."
mkdir -p /etc/wireguard
printf '%s\n' "$WG_CONFIG" > /etc/wireguard/wg0.conf
chmod 600 /etc/wireguard/wg0.conf

echo "Bringing up wg0..."
wg-quick up wg0

echo "=== IPv6 Diagnostics ==="
echo "--- ip a ---"
ip a
echo ""
echo "--- wg-quick journalctl ---"
journalctl -u wg-quick@wg0 --no-pager || echo "WARNING: could not retrieve wg-quick journal logs"
echo ""
echo "--- curl -6 ghcr.io ---"
curl -6 -L https://ghcr.io/v2/ || echo "WARNING: IPv6 curl to ghcr.io failed"
echo ""
echo "WireGuard setup complete."
