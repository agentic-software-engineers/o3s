#!/bin/bash
# Layout siblings, run on the runner: the measured VM beside a second VM of the same size, as the gateway VM sits
set -uo pipefail
. "$(dirname "$0")/lib.sh"
. "$(dirname "$0")/boot.sh"

gate "the gateway VM starts" limactl start --name=gateway --vm-type=qemu --cpus=2 --memory=6 --timeout=30m --tty=false internal/vm/o3s.yaml
boot_times internal/vm/o3s.yaml
gate "the measured VM starts beside the gateway VM" limactl start --name=cage --vm-type=qemu --cpus=2 --memory=6 --timeout=30m --tty=false internal/vm/o3s.yaml
limactl copy -r spike cage:/tmp/
limactl shell cage -- bash /tmp/spike/bench.sh
free -m | awk '/^Mem:/ {print "BENCH | runner memory used | " $3 " | MiB |"}'
