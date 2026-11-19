#!/bin/bash
# Layout N, run on the runner: the cage VM inside the gateway VM
set -uo pipefail
. "$(dirname "$0")/lib.sh"

cat /sys/module/kvm_amd/parameters/nested /sys/module/kvm_intel/parameters/nested 2>/dev/null
gate "the runner's KVM offers nesting" grep -qxE 'Y|1' /sys/module/kvm_amd/parameters/nested /sys/module/kvm_intel/parameters/nested
gate "the gateway VM starts with nested virtualization" limactl start --name=gateway --vm-type=qemu --cpus=3 --memory=10 \
  --set '.nestedVirtualization = true' --timeout=30m --tty=false internal/vm/o3s.yaml
limactl copy -r spike internal/vm/o3s.yaml gateway:/tmp/
check "the gateway VM runs the cage VM" limactl shell gateway -- bash /tmp/spike/nested-gateway.sh
report
