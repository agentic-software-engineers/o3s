#!/bin/bash
# Spike V on Windows: the cage VM under the Host Compute Service with no network adapter, linked to the host over Hyper-V sockets
set -uo pipefail
. "$(dirname "$0")/lib.sh"

SPIKE="$(cd "$(dirname "$0")" && pwd)"
WORK="$(cygpath -u "${RUNNER_TEMP}")/vsock"
WIN="$(cygpath -m "${WORK}")"
PROXY_LOG="${WORK}/proxy.log"

# Ssh into the cage VM over a Hyper-V socket to guest port 2222
cage() {
  ssh -i "${WORK}/key" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o ConnectTimeout=10 \
    -o ProxyCommand="${WIN}/vsockhost.exe stdio ${WIN} 2222" ubuntu@cage "$@"
}

# The disk and seed come from the prepare job; HCS opens a disk only without the sparse flag
cp "${SPIKE}/vsock-allow.txt" "${WORK}/allow.txt"
chmod 600 "${WORK}/key"
fsutil sparse setflag "${WIN}/disk.vhdx" 0

# Boot the cage VM with no network adapter, and serve the allow-list proxy on its vsock port 3128
START=${SECONDS}
"${WORK}/vsockhost.exe" run "${WIN}" > "${WORK}/hcsvm.log" 2>&1 &
. "${SPIKE}/vsock-checks.sh"
echo "--- hcsvm"
tail -20 "${WORK}/hcsvm.log"
echo "--- console"
tail -40 "${WORK}/console.log"
