#!/usr/bin/env bash
#
# NAME
#        traffic.sh — record the gateway's traffic per allowed host and the names it refused
#
# DESCRIPTION
#        Runs beside the resolver for the gateway's lifetime. Every interval it reads the bytes
#        each allowed host moved from the counting chain and the names the resolver refused from
#        its query log, adds them to what earlier lifetimes recorded, and writes the total to the
#        gateway's state volume, where o3s asks for it.
#
# SEE ALSO
#        entrypoint.sh, o3s.json

set -uo pipefail

# Count lengths in bytes, as the log offset needs.
export LC_ALL=C

# Fixed image contract for the record, the policy, the query log and the counting chain.
STATE_DIR=/state
RECORD="$STATE_DIR/traffic.json"
CONFIG_FILE=/config/o3s.json
DNS_LOG=/var/log/o3s/dns.log
CHAIN=O3S_ACCT

# How often the record is written, and how long a refused name stays in it.
INTERVAL="${O3S_TRAFFIC_INTERVAL:-60}"
BLOCKED_KEEP=$((30 * 24 * 3600))
# The query log is rotated once it grows past this size.
LOG_MAX=$((1024 * 1024))

declare -A BYTES=()       # bytes per host, earlier lifetimes included
declare -A SEEN=()        # when a host last moved bytes
declare -A TRACKED=()     # when the gateway first counted a host
declare -A COUNTED=()     # bytes per host the counting chain reported last time
declare -A ATTEMPTS=()    # refused lookups per name
declare -A REFUSED_AT=()  # when a name was last refused
declare -A ASKED=()       # names asked for, by the resolver's query serial
declare -A ASKER=()       # the socket that asked, by the resolver's query serial
declare -A LAST_ASKER=()  # the socket a name was last refused to, so its retries count once
SINCE=""
OFFSET=0

# Every host the policy allows, one per line.
allowed_hosts() {
  yq -p=json -o=tsv '.hosts[].name' "$CONFIG_FILE" 2>/dev/null || true
}

# Start from what earlier lifetimes recorded, keeping allowed hosts and recent refusals alone.
load_record() {
  local now allowed name bytes seen since attempts at
  now="$(date +%s)"
  SINCE="$now"
  allowed="$(allowed_hosts)"
  # Start empty where an earlier record is absent
  [ -f "$RECORD" ] || return 0
  SINCE="$(yq -p=json '.since // 0' "$RECORD" 2>/dev/null || echo 0)"
  # Take the tracking start as now where the record lacks one
  [ "$SINCE" -gt 0 ] 2>/dev/null || SINCE="$now"
  # Carry over every host the policy still allows
  while IFS=$'\t' read -r name bytes seen since; do
    # Keep a host the policy allows
    if [ -n "$name" ] && grep -qxF "$name" <<<"$allowed"; then
      BYTES[$name]="$bytes"
      SEEN[$name]="$seen"
      TRACKED[$name]="$since"
    fi
  done < <(yq -p=json -o=tsv '.hosts // {} | to_entries | .[] | [.key, .value.bytes, .value.last_seen, (.value.since // 0)]' "$RECORD" 2>/dev/null)
  # Carry over every recent refusal of a name the policy still leaves out
  while IFS=$'\t' read -r name attempts at; do
    # Keep a name still refused and refused lately
    if [ -n "$name" ] && ! grep -qxF "$name" <<<"$allowed" && [ $((now - at)) -lt "$BLOCKED_KEEP" ]; then
      ATTEMPTS[$name]="$attempts"
      REFUSED_AT[$name]="$at"
    fi
  done < <(yq -p=json -o=tsv '.blocked // {} | to_entries | .[] | [.key, .value.attempts, .value.last_seen]' "$RECORD" 2>/dev/null)
}

# Add the bytes each host moved since the last reading, noting when it moved any.
read_counters() {
  local now="$1" bytes name
  declare -A current=()
  # Sum both directions of every host the chain counts
  while read -r bytes name; do
    current[$name]=$(( ${current[$name]:-0} + bytes ))
  done < <(iptables -L "$CHAIN" -v -x -n 2>/dev/null | awk 'match($0, /\/\* .* \*\//) { print $2, substr($0, RSTART + 3, RLENGTH - 6) }')
  # Credit every host with what it moved since the last reading
  for name in "${!current[@]}"; do
    local grown=$(( current[$name] - ${COUNTED[$name]:-0} ))
    # Note a host that moved bytes
    if [ "$grown" -gt 0 ]; then
      BYTES[$name]=$(( ${BYTES[$name]:-0} + grown ))
      SEEN[$name]="$now"
    fi
    COUNTED[$name]="${current[$name]}"
    : "${BYTES[$name]:=0}" "${SEEN[$name]:=0}"
    # Start tracking a host the first time the chain counts it
    if [ "${TRACKED[$name]:-0}" -le 0 ] 2>/dev/null; then
      TRACKED[$name]="$now"
    fi
  done
}

# Count the names the resolver refused since the last reading, pairing each refusal with its query.
read_refusals() {
  local now="$1" size chunk complete line serial name
  # Leave the log alone until the resolver writes one
  [ -f "$DNS_LOG" ] || return 0
  size="$(stat -c %s "$DNS_LOG" 2>/dev/null || echo 0)"
  # Wait for lines the last reading left unread
  [ "$size" -gt "$OFFSET" ] || return 0
  chunk="$(tail -c +"$((OFFSET + 1))" "$DNS_LOG" | head -c "$((size - OFFSET))"; printf x)"
  chunk="${chunk%x}"
  # Leave a line the resolver is still writing for the next reading
  [[ "$chunk" == *$'\n'* ]] || return 0
  complete="${chunk%$'\n'*}"
  OFFSET=$(( OFFSET + ${#complete} + 1 ))
  # Read every complete new line of the log
  while IFS= read -r line; do
    # Remember the name an address query asks for, by its serial
    if [[ "$line" =~ \]:\ ([0-9]+)\ ([^\ ]+)\ query\[A\]\ ([^\ ]+)\ from ]]; then
      ASKED[${BASH_REMATCH[1]}]="${BASH_REMATCH[3]}"
      ASKER[${BASH_REMATCH[1]}]="${BASH_REMATCH[2]}"
    # Count the refusal of a name asked for
    elif [[ "$line" =~ \]:\ ([0-9]+)\ [^\ ]+\ config\ error\ is\ REFUSED ]]; then
      serial="${BASH_REMATCH[1]}"
      name="${ASKED[$serial]:-}"
      # Count a refusal whose query is known, once per socket that asked
      if [ -n "$name" ] && [ "${LAST_ASKER[$name]:-}" != "${ASKER[$serial]}" ]; then
        ATTEMPTS[$name]=$(( ${ATTEMPTS[$name]:-0} + 1 ))
        LAST_ASKER[$name]="${ASKER[$serial]}"
      fi
      # Note when a known name was refused
      if [ -n "$name" ]; then
        REFUSED_AT[$name]="$now"
      fi
      unset "ASKED[$serial]" "ASKER[$serial]"
    fi
  done <<<"$complete"
  # Rotate a log grown past its limit, so the resolver starts a new one
  if [ "$size" -gt "$LOG_MAX" ]; then
    mv "$DNS_LOG" "$DNS_LOG.1"
    pkill -USR2 -x dnsmasq || true
    OFFSET=0
  fi
}

# Write the record in a single step.
write_record() {
  local now="$1" tmp="$RECORD.tmp" name sep
  {
    printf '{\n  "since": %s,\n  "updated": %s,\n  "hosts": {' "$SINCE" "$now"
    sep=""
    # Write every host with its bytes and when it last moved any
    for name in "${!BYTES[@]}"; do
      printf '%s\n    "%s": {"bytes": %s, "last_seen": %s, "since": %s}' "$sep" "$name" "${BYTES[$name]}" "${SEEN[$name]:-0}" "${TRACKED[$name]:-$now}"
      sep=","
    done
    printf '\n  },\n  "blocked": {'
    sep=""
    # Write every refused name with its attempts and when it was last refused
    for name in "${!ATTEMPTS[@]}"; do
      printf '%s\n    "%s": {"attempts": %s, "last_seen": %s}' "$sep" "$name" "${ATTEMPTS[$name]}" "${REFUSED_AT[$name]}"
      sep=","
    done
    printf '\n  }\n}\n'
  } > "$tmp"
  chmod 0644 "$tmp"
  mv "$tmp" "$RECORD"
}

# Skip recording where the state volume is absent
[ -d "$STATE_DIR" ] || exit 0

load_record
# Record the traffic once per interval for the gateway's lifetime
while sleep "$INTERVAL"; do
  now="$(date +%s)"
  read_counters "$now"
  read_refusals "$now"
  write_record "$now"
done
