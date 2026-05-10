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
# Configure DNS64 (Unbound) + NAT64 (TAYGA) on a GitHub Actions runner
# to provide fake IPv6 internet connectivity. This must run before cluster.sh.
#
# Network flow after setup:
#   DNS query  -> Unbound (dns64-synthall) -> AAAA 64:ff9b::<ipv4>
#   IPv6 packet to 64:ff9b::/96 -> TAYGA -> IPv4 -> internet
#

set -o errexit
set -o nounset
set -o pipefail

source "$(cd "$(dirname "$0")" && pwd)/common.sh"

readonly NAT64_PREFIX="64:ff9b::/96"
readonly TAYGA_IPV4="192.168.255.1"
readonly TAYGA_POOL="192.168.255.0/24"
readonly TAYGA_DEV="nat64"

main() {
  echo "${blue}Configuring DNS64 + NAT64${reset}"

  local upstream_dns
  upstream_dns="$(get_upstream_dns)"
  echo "Upstream DNS: ${upstream_dns}"

  install_packages
  configure_dns64 "${upstream_dns}"
  configure_nat64
  verify

  echo "${green}✅ DNS64 + NAT64${reset}"
}

get_upstream_dns() {
  local dns=""
  if [[ -f /run/systemd/resolve/resolv.conf ]]; then
    dns="$(awk '/^nameserver/ { print $2; exit }' /run/systemd/resolve/resolv.conf)"
  fi
  if [[ -z "${dns}" ]]; then
    dns="8.8.8.8"
  fi
  echo "${dns}"
}

install_packages() {
  echo "${blue}Installing unbound and tayga${reset}"
  sudo apt-get update -qq
  sudo apt-get install -y -qq unbound tayga
}

configure_dns64() {
  local upstream_dns="$1"

  echo "${blue}Configuring Unbound (DNS64)${reset}"

  # Stop systemd-resolved so we can take over port 53
  sudo systemctl stop systemd-resolved || true
  sudo systemctl disable systemd-resolved || true

  # Point the system resolver at our local Unbound
  sudo tee /etc/resolv.conf > /dev/null <<EOF
nameserver 127.0.0.1
EOF

  # Write Unbound config with DNS64 synthesis
  sudo tee /etc/unbound/unbound.conf > /dev/null <<EOF
server:
  interface: 127.0.0.1
  interface: ::1
  access-control: 127.0.0.0/8 allow
  access-control: ::1/128 allow

  module-config: "dns64 iterator"
  dns64-prefix: ${NAT64_PREFIX}
  dns64-synthall: yes

  # Serve localhost records so local services resolve correctly
  local-zone: "localhost." static
  local-data: "localhost. IN A 127.0.0.1"
  local-data: "localhost. IN AAAA ::1"

forward-zone:
  name: "."
  forward-addr: ${upstream_dns}
EOF

  sudo systemctl restart unbound
  sudo systemctl enable unbound

  echo "${green}Unbound configured${reset}"
}

configure_nat64() {
  echo "${blue}Configuring TAYGA (NAT64)${reset}"

  # Write TAYGA config
  sudo tee /etc/tayga.conf > /dev/null <<EOF
tun-device ${TAYGA_DEV}
prefix ${NAT64_PREFIX}
ipv4-addr ${TAYGA_IPV4}
dynamic-pool ${TAYGA_POOL}
data-dir /var/db/tayga
EOF

  sudo mkdir -p /var/db/tayga

  # Create the TUN device and configure it
  sudo tayga --mktun
  sudo ip link set ${TAYGA_DEV} up
  sudo ip addr add ${TAYGA_IPV4} dev ${TAYGA_DEV}
  sudo ip addr add 2001:db8::1 dev ${TAYGA_DEV}
  sudo ip route add ${TAYGA_POOL} dev ${TAYGA_DEV}
  sudo ip route add ${NAT64_PREFIX} dev ${TAYGA_DEV}

  # Enable forwarding
  sudo sysctl -w net.ipv4.ip_forward=1
  sudo sysctl -w net.ipv6.conf.all.forwarding=1

  # NAT the TAYGA pool so translated packets can reach the internet
  sudo iptables -t nat -A POSTROUTING -s ${TAYGA_POOL} -j MASQUERADE

  # Start TAYGA
  sudo tayga

  echo "${green}TAYGA configured${reset}"
}

verify() {
  echo "${blue}Verifying DNS64 + NAT64${reset}"

  # Verify DNS64 synthesis
  local aaaa
  aaaa="$(dig +short AAAA github.com @127.0.0.1)"
  if [[ "${aaaa}" != *"64:ff9b::"* ]]; then
    echo "${red}DNS64 verification failed: expected 64:ff9b:: prefix in AAAA record${reset}"
    echo "Got: ${aaaa}"
    exit 1
  fi
  echo "DNS64 OK: github.com -> ${aaaa}"

  # Verify NAT64 connectivity
  if ! curl -6 --max-time 10 -sSf -o /dev/null https://github.com; then
    echo "${red}NAT64 verification failed: cannot reach github.com over IPv6${reset}"
    exit 1
  fi
  echo "NAT64 OK: curl -6 github.com succeeded"
}

main "$@"
