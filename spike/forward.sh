#!/bin/bash
# Layout S, run as root in the gateway VM: resolve and forward the cage VM's allowed traffic out of the gateway VM
set -euo pipefail
. "$(dirname "$0")/lib.sh"

ip=$(ip -4 -o addr show dev lima0 | awk '{print $4}' | cut -d/ -f1)
apt-get update -q && apt-get install -y -q --no-install-recommends dnsmasq-base
# Answer the cage VM's lookups through this VM's resolver, so both sides see the same addresses
dnsmasq --interface=lima0 --bind-interfaces --no-resolv --no-hosts --server=127.0.0.53 --listen-address="${ip}"
sysctl -w net.ipv4.ip_forward=1
iptables -t nat -A POSTROUTING -s 192.168.106.0/24 -o eth0 -j MASQUERADE
iptables -N cage-forward
iptables -A cage-forward -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
# Open each allowed address to HTTPS
for addr in $(allowed_ips); do
  iptables -A cage-forward -i lima0 -o eth0 -d "${addr}" -p tcp --dport 443 -j ACCEPT
done
iptables -A cage-forward -i lima0 -j LOG --log-prefix "forward-refused "
iptables -A cage-forward -i lima0 -j REJECT
iptables -I FORWARD 1 -j cage-forward
