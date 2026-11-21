#!/bin/bash
# Spike V on macOS: the cage VM under Virtualization.framework through vfkit, with no network device, linked to the host over vsock
set -uo pipefail
. "$(dirname "$0")/lib.sh"

SPIKE="$(cd "$(dirname "$0")" && pwd)"
WORK="${RUNNER_TEMP:-/tmp}/vsock"
PROXY_LOG="${WORK}/proxy.log"

# Ssh into the cage VM through vfkit's socket for guest port 2222
cage() {
  ssh -i "${WORK}/key" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o ConnectTimeout=10 \
    -o ProxyCommand="python3 ${SPIKE}/vsock-relay.py stdio-unix ${WORK}/v2222.sock" ubuntu@cage "$@"
}

# The raw disk the prepare job converted from the pinned image, grown to 30 GiB
zstd -q -d "${WORK}/disk.raw.zst" -o "${WORK}/disk.raw"
python3 -c 'import os, sys; os.truncate(sys.argv[1], 30 << 30)' "${WORK}/disk.raw"

# The allow-list proxy, served on the socket vfkit dials for each guest connection to host port 3128
"${WORK}/vsockhost" proxy-unix "${WORK}/v3128.sock" "${SPIKE}/vsock-allow.txt" "${PROXY_LOG}" &

bash "${SPIKE}/vsock-seed.sh" "${WORK}"
"${WORK}/vfkit" --version

# Boot the cage VM with no network device: vsock 3128 out to the host, vsock 2222 in from the host
START=${SECONDS}
"${WORK}/vfkit" --cpus 2 --memory 6144 \
  --bootloader "efi,variable-store=${WORK}/efi-vars,create" \
  --device "virtio-blk,path=${WORK}/disk.raw" \
  --cloud-init "${WORK}/user-data,${WORK}/meta-data" \
  --device "virtio-vsock,port=3128,socketURL=${WORK}/v3128.sock" \
  --device "virtio-vsock,port=2222,socketURL=${WORK}/v2222.sock,connect" \
  --device "virtio-serial,logFilePath=${WORK}/console.log" > "${WORK}/vfkit.log" 2>&1 &
. "${SPIKE}/vsock-checks.sh"
echo "--- vfkit"
tail -20 "${WORK}/vfkit.log"
echo "--- console"
tail -40 "${WORK}/console.log"
