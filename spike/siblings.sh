#!/bin/bash
# Layout S, run on the runner: the cage VM beside the gateway VM, joined by Lima's host network
set -uo pipefail
. "$(dirname "$0")/lib.sh"

# Pass frames between the taps on Lima's host bridge, which the engine's bridge filtering would otherwise hand to its FORWARD policy
sudo iptables -I FORWARD 1 -i lima-host -o lima-host -j ACCEPT
gate "the gateway VM starts on the host network" limactl start --name=gateway --vm-type=qemu --cpus=2 --memory=6 \
  --network=lima:host --timeout=30m --tty=false internal/vm/o3s.yaml
limactl copy -r spike gateway:/tmp/
gate "the gateway VM forwards the host network" limactl shell gateway -- sudo bash /tmp/spike/forward.sh
gw=$(limactl shell gateway -- ip -4 -o addr show dev lima0 | awk '{print $4}' | cut -d/ -f1)
echo "gateway VM on the host network: ${gw}"
gate "the cage VM starts beside the gateway VM" start_fenced cage --cpus=2 --memory=6 --network=lima:host \
  --set '.hostResolver.enabled = false' internal/vm/o3s.yaml
check "the cage VM runs on KVM in its scope" on_kvm_in_scope cage
install_kind cage
# Route the cage VM and its resolver through the gateway VM
limactl shell cage -- sudo sh -c "ip route replace default via ${gw} dev lima0 metric 50 && rm -f /etc/resolv.conf && echo 'nameserver ${gw}' > /etc/resolv.conf"
fence cage
check "the cage reaches the gateway VM" limactl shell cage -- ping -c 3 -W 5 "${gw}"
workload_checks cage
check "the cage is refused example.com over its own NIC" fails limactl shell cage -- curl -sS -o /dev/null --max-time 20 --interface eth0 https://example.com
limactl shell gateway -- sudo iptables -L cage-forward -v -n
report
