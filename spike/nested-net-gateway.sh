#!/bin/bash
# Spike N2, run in the gateway VM: start the cage VM, then steer the traffic of its scope into a resolver that admits
# the allow-list alone and into a transparent proxy, the way the gateway steers the cage today
set -uo pipefail
. "$(dirname "$0")/lib.sh"

SCOPE="system.slice/cage.scope"
# The names the cage reaches: the forge it proves the proxy with, and the registry kind pulls its node image from
ALLOWED=(github.com docker.io docker.com cloudflarestorage.com amazonaws.com)
MITM=ghcr.io/mitmproxy/mitmproxy:12.2.3@sha256:00b77b5d8804c8ad18cb6caefbf9d5849e895e8986c5ce011f4ae30f4385962f

sudo apt-get update -q >/dev/null && sudo apt-get install -y -q --no-install-recommends ovmf qemu-system-x86 qemu-utils dnsmasq-base ipset >/dev/null
sudo chown "${USER}" /dev/kvm
bash "$(dirname "$0")/install-lima.sh"
gate "the cage VM starts inside the gateway VM" start_fenced cage --cpus=2 --memory=6 /tmp/o3s.yaml
check "the cage VM runs on KVM in its scope" on_kvm_in_scope cage
install_kind cage

# Admit the allowed names alone: the resolver answers them and files their addresses in the allow set, those the proxy
# handles in the proxied set too, and refuses the rest
sudo ipset create allow hash:ip timeout 3600
sudo ipset create proxied hash:ip timeout 3600
domains="$(printf '/%s' "${ALLOWED[@]}")"
direct="$(printf '/%s' "${ALLOWED[@]:1}")"
sudo dnsmasq --port=5353 --listen-address=127.0.0.1 --bind-interfaces --no-resolv --no-hosts \
  --server="${domains}/1.1.1.1" --ipset="${direct}/allow" --ipset=/github.com/allow,proxied --address=/#/ \
  --log-queries --log-facility=/tmp/dnsmasq.log
# Take the cage's HTTPS on the proxy, which reads where each connection was headed
docker run -d --name proxy --network host --entrypoint mitmdump "${MITM}" --mode transparent --listen-port 8443 \
  --showhost --set block_global=false >/dev/null
check "the proxy listens on 8443" timeout 120 sh -c 'until ss -ltn | grep -q ":8443 "; do sleep 2; done'
docker ps -a --filter name=proxy
docker logs proxy 2>&1 | tail -20

# Steer the scope's lookups to the resolver and its HTTPS to the proxied hosts to the proxy
sudo iptables -t nat -A OUTPUT -m cgroup --path "${SCOPE}" -p udp --dport 53 -j REDIRECT --to-ports 5353
sudo iptables -t nat -A OUTPUT -m cgroup --path "${SCOPE}" -p tcp --dport 53 -j REDIRECT --to-ports 5353
sudo iptables -t nat -A OUTPUT -m cgroup --path "${SCOPE}" -p tcp --dport 443 -m set --match-set proxied dst -j REDIRECT --to-ports 8443
# Admit loopback, where the steered traffic and the VM manager's own channels land, then the allowed addresses, and
# refuse the rest
sudo iptables -N cage
sudo iptables -A cage -o lo -j ACCEPT
sudo iptables -A cage -m set --match-set allow dst -j ACCEPT
sudo iptables -A cage -j LOG --log-prefix "cage-refused "
sudo iptables -A cage -j REJECT
sudo iptables -I OUTPUT 1 -m cgroup --path "${SCOPE}" -j cage
# Drop the names the cage VM resolved before the fence, so every lookup from here on passes the allow-list
limactl shell cage -- sudo resolvectl flush-caches
sleep 5

check "the cage reaches github.com through the proxy" limactl shell cage -- curl -sSk -o /dev/null --max-time 20 https://github.com
check "the proxy saw the cage's github.com request" bash -c "docker logs proxy 2>&1 | grep -q github.com"
check "the cage gets no answer for example.com" fails limactl shell cage -- getent hosts example.com
ip="$(getent ahostsv4 example.com | awk 'NR == 1 {print $1}')"
check "the cage is refused example.com by address" fails limactl shell cage -- curl -sSk -o /dev/null --max-time 20 --resolve "example.com:443:${ip}" https://example.com
check "the cage runs hello-world" limactl shell cage -- docker run --rm hello-world
check "the cage raises a kind cluster" limactl shell cage -- kind create cluster --wait 5m
sudo tail -20 /tmp/dnsmasq.log
sudo journalctl -k --no-pager | grep cage-refused | tail -10 || true
echo "--- proxy at the end"
docker ps -a --filter name=proxy
docker logs proxy 2>&1 | tail -30
sudo iptables -t nat -L OUTPUT -v -n
sudo iptables -L cage -v -n
sudo ipset list proxied | tail -5
