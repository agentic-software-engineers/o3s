#!/usr/bin/env bash
#
# NAME
#        check-egress.sh — probe that the egress policy holds
#
# DESCRIPTION
#        Reports on every container start that a direct connection stays refused and that the
#        proxy turns away a host outside the allowlist.
#
# SEE ALSO
#        devcontainer.json, o3s init

set -euo pipefail

DENY_HOST=1.1.1.1
PROXY_PORT=3128
REFUSED_NAME=o3s.invalid

# A direct connection to a public address stays refused, since the cage holds the loopback alone
if curl --noproxy '*' -k -s --connect-timeout 3 -o /dev/null "https://$DENY_HOST"; then
  echo "[o3s] WARNING: the cage reaches the internet directly, past the host's allowlist" >&2
  exit 0
fi

# Give the bridge its moment to open the proxy port, since the editor runs this check as the cage starts
for _ in $(seq 30); do
  (exec 3<>"/dev/tcp/127.0.0.1/$PROXY_PORT") 2>/dev/null && break
  sleep 1
done

# The proxy answers and turns away a host outside the allowlist
status=""
if exec 3<>"/dev/tcp/127.0.0.1/$PROXY_PORT"; then
  printf 'CONNECT %s:443 HTTP/1.1\r\nHost: %s:443\r\n\r\n' "$REFUSED_NAME" "$REFUSED_NAME" >&3
  read -r -t 5 _ status _ <&3 || true
  exec 3>&-
fi
if [ "$status" = 403 ]; then
  echo "[o3s] INFO: network lockdown active, only allowlisted hosts are reachable"
else
  echo "[o3s] WARNING: the proxy stays silent, so the cage reaches no host; o3s host start brings it back" >&2
fi
