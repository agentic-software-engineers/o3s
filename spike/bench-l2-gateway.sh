#!/bin/bash
# Layout l2, run in the gateway VM: start the measured VM inside and measure it
set -uo pipefail
. "$(dirname "$0")/lib.sh"
. "$(dirname "$0")/boot.sh"

gate "the gateway VM holds /dev/kvm" test -e /dev/kvm
sudo apt-get update -q >/dev/null && sudo apt-get install -y -q --no-install-recommends ovmf qemu-system-x86 qemu-utils >/dev/null
sudo chown "${USER}" /dev/kvm
bash "$(dirname "$0")/install-lima.sh"
boot_times /tmp/o3s.yaml
gate "the measured VM starts inside the gateway VM" limactl start --name=cage --vm-type=qemu --cpus=2 --memory=6 --timeout=30m --tty=false /tmp/o3s.yaml
limactl copy -r /tmp/spike cage:/tmp/
limactl shell cage -- bash /tmp/spike/bench.sh
