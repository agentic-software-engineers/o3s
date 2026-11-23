#!/usr/bin/env bash
#
# NAME
#        start.sh — bring the cage up with its bridge to the host's sockets
#
# DESCRIPTION
#        The cage container's command: trusts this host's proxy CA, exports the markers the
#        allowlist declares, then runs o3s-cage, which carries the cage's proxy and DNS ports to
#        the host's sockets for as long as the container runs.
#
# SEE ALSO
#        compose.yaml, markers.sh

set -euo pipefail

# This script's own directory, holding the helpers it calls
CAGE_DIR="$(dirname "$0")"

# Trust this host's proxy CA
update-ca-certificates >/dev/null 2>&1 || true

# Export the markers the allowlist declares
bash "$CAGE_DIR/markers.sh" || true

# Carry the proxy and DNS ports to the host's sockets, holding the container open
exec o3s-cage serve -sockets /run/o3s
