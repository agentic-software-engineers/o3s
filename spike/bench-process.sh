#!/bin/bash
# Layout process, run on the runner: the measurements straight on the runner's own engine, with 2 threads like the VMs
set -uo pipefail

BENCH_THREADS=2 bash "$(dirname "$0")/bench.sh"
free -m | awk '/^Mem:/ {print "BENCH | runner memory used | " $3 " | MiB |"}'
