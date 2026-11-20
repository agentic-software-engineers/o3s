#!/bin/bash
# Layout l1, run on the runner: the measured VM straight on the runner
set -uo pipefail
. "$(dirname "$0")/lib.sh"
. "$(dirname "$0")/boot.sh"

cat /sys/module/kvm_amd/parameters/npt /sys/module/kvm_amd/parameters/nested 2>/dev/null
boot_times internal/vm/o3s.yaml
gate "the measured VM starts" limactl start --name=cage --vm-type=qemu --cpus=2 --memory=6 --timeout=30m --tty=false internal/vm/o3s.yaml
limactl copy -r spike cage:/tmp/
limactl shell cage -- bash /tmp/spike/bench.sh
free -m | awk '/^Mem:/ {print "BENCH | runner memory used | " $3 " | MiB |"}'
