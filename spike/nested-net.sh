#!/bin/bash
# Spike N2, run on the runner: the cage VM inside the gateway VM, its traffic steered into the gateway's allow-list
set -uo pipefail
. "$(dirname "$0")/lib.sh"

gate "the gateway VM starts with nested virtualization" limactl start --name=gateway --vm-type=qemu --cpus=3 --memory=10 \
  --set '.nestedVirtualization = true' --timeout=30m --tty=false internal/vm/o3s.yaml
limactl copy -r spike internal/vm/o3s.yaml gateway:/tmp/
check "the gateway VM steers the cage VM" limactl shell gateway -- bash /tmp/spike/nested-net-gateway.sh
