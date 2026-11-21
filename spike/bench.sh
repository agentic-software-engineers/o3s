#!/bin/bash
# Measure one VM from inside it: fingerprint, CPU, disk, sync latency and kind, repeated, one BENCH row per run
set -uo pipefail

REPEATS=3
# The threads the CPU and disk tests run, matching the measured VMs' 2 vCPU where the layout runs on more
THREADS="${BENCH_THREADS:-$(nproc)}"
NODE=kindest/node:v1.37.0@sha256:a1ed56cfb0e7b93589bdf97c8cd566405a265939e3620fc4f5de89adff580ae5

# Print one result row
row() {
  echo "BENCH | $1 | $2 | $3 |"
}

# Name the virtualization depth and the nested paging this layer runs on
systemd-detect-virt || true
nproc
free -m
lscpu | grep -iE 'model name|hypervisor|virtualization' || true
cat /sys/module/kvm_amd/parameters/npt /sys/module/kvm_amd/parameters/nested 2>/dev/null || true

sudo apt-get update -q >/dev/null && sudo apt-get install -y -q --no-install-recommends fio sysbench jq >/dev/null
sudo curl -fsSLo /usr/local/bin/kind https://github.com/kubernetes-sigs/kind/releases/download/v0.33.0/kind-linux-amd64
echo "aee6151561422756b764a4ae28e7f44cda5af5a9eead3cc9985112b1de8d8e0d  /usr/local/bin/kind" | sha256sum -c
sudo chmod +x /usr/local/bin/kind
docker pull -q "${NODE}" >/dev/null

# Run fio from a job's arguments and print the field the jq filter picks
fio_json() {
  local filter=$1
  shift
  sync && echo 3 | sudo tee /proc/sys/vm/drop_caches >/dev/null
  sudo fio --output-format=json "$@" | jq -r "${filter}"
  sudo rm -f /var/tmp/fio.dat
}

for _ in $(seq "${REPEATS}"); do
  # CPU on one thread and on every thread, in events per second
  row "cpu 1 thread" "$(sysbench cpu --threads=1 --time=20 run | awk '/events per second/ {print $4}')" "events/s"
  row "cpu all threads" "$(sysbench cpu --threads="${THREADS}" --time=20 run | awk '/events per second/ {print $4}')" "events/s"
  # Random 4k reads and writes, as Firecracker's block test runs them
  for rw in randread randwrite; do
    row "disk ${rw} 4k" "$(fio_json '.jobs[0] | (.read.iops + .write.iops) | floor' --name=r --filename=/var/tmp/fio.dat --size=2G \
      --bs=4k --rw="${rw}" --direct=1 --ioengine=libaio --iodepth=32 --numjobs="${THREADS}" --time_based=1 --runtime=20 \
      --ramp_time=5 --randrepeat=0 --group_reporting)" "iops"
  done
  # Sequential 1M reads and writes
  for rw in read write; do
    row "disk ${rw} 1M" "$(fio_json '.jobs[0] | ((.read.bw + .write.bw) / 1024) | floor' --name=s --filename=/var/tmp/fio.dat --size=2G \
      --bs=1M --rw="${rw}" --direct=1 --ioengine=libaio --iodepth=8 --numjobs=1 --time_based=1 --runtime=20 --ramp_time=5)" "MiB/s"
  done
  # Writes each followed by fdatasync, the load etcd puts on the disk
  row "disk fdatasync p99" "$(fio_json '.jobs[0].sync.lat_ns.percentile."99.000000" / 1000 | floor' --name=f \
    --filename=/var/tmp/fio.dat --rw=write --ioengine=sync --fdatasync=1 --bs=2300 --size=22m)" "us"
  # A kind cluster from the preloaded node image, until its node is Ready
  start=$(date +%s.%N)
  kind create cluster --image "${NODE}" --wait 5m >/dev/null 2>&1 && ok=1 || ok=0
  end=$(date +%s.%N)
  row "kind create to Ready" "$( [ "${ok}" = 1 ] && awk "BEGIN {print ${end} - ${start}}" || echo FAIL)" "s"
  free -m | awk '/^Mem:/ {print "BENCH | memory used with kind | " $3 " | MiB |"}'
  kind delete cluster >/dev/null 2>&1
done
