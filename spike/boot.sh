#!/bin/bash
# Time a bare boot of the template's image, without its provisioning, three times after a first boot that downloads it
boot_times() {
  local template=$1 run start end label
  for run in 0 1 2 3; do
    start=$(date +%s.%N)
    limactl start --name="boot${run}" --vm-type=qemu --cpus=2 --memory=6 --plain --timeout=30m --tty=false \
      --set '.provision = [] | .probes = []' "${template}" >/dev/null 2>&1
    end=$(date +%s.%N)
    # Label the first boot apart, since it also downloads the image
    label="boot without provisioning"
    [ "${run}" = 0 ] && label="first boot with image download"
    echo "BENCH | ${label} | $(awk "BEGIN {print ${end} - ${start}}") | s |"
    limactl delete --force "boot${run}" >/dev/null 2>&1
  done
}
