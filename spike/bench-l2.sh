#!/bin/bash
# Layout l2, run on the runner: the measured VM inside a gateway VM
set -uo pipefail
. "$(dirname "$0")/lib.sh"

cat /sys/module/kvm_amd/parameters/npt /sys/module/kvm_amd/parameters/nested 2>/dev/null
gate "the gateway VM starts with nested virtualization" limactl start --name=gateway --vm-type=qemu --cpus=3 --memory=9 \
  --set '.nestedVirtualization = true' --timeout=30m --tty=false internal/vm/o3s.yaml
limactl copy -r spike internal/vm/o3s.yaml gateway:/tmp/
limactl shell gateway -- bash /tmp/spike/bench-l2-gateway.sh
free -m | awk '/^Mem:/ {print "BENCH | runner memory used | " $3 " | MiB |"}'
