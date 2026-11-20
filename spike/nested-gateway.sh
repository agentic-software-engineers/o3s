#!/bin/bash
# Layout N, run in the gateway VM: start the cage VM inside, fence its traffic, and check it
set -uo pipefail
. "$(dirname "$0")/lib.sh"

gate "the gateway VM holds /dev/kvm" test -e /dev/kvm
check "the gateway VM's CPU offers virtualization" grep -qE 'svm|vmx' /proc/cpuinfo
sudo apt-get update -q && sudo apt-get install -y -q --no-install-recommends ovmf qemu-system-x86 qemu-utils
sudo chown "${USER}" /dev/kvm
bash "$(dirname "$0")/install-lima.sh"
gate "the cage VM starts inside the gateway VM" start_fenced cage --cpus=2 --memory=6 /tmp/o3s.yaml
check "the cage VM runs on KVM in its scope" on_kvm_in_scope cage
install_kind cage
mapfile -t ips < <(allowed_ips)
fence cage "${ips[@]}"
egress_checks cage
# Open HTTPS to every host, since the registries serve their layers from storage addresses that change per request
sudo iptables -I fence 2 -p tcp --dport 443 -j ACCEPT
workload_checks cage
report
