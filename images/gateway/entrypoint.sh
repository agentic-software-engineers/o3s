#!/usr/bin/env bash
#
# NAME
#        entrypoint.sh — build the gateway's egress policy and run its resolver
#
# DESCRIPTION
#        The gateway's PID 1: fails closed, locates the cage and egress interfaces, turns the
#        allowlist into ipsets and iptables rules, diverts cage HTTPS to the proxy, counts each
#        allowed host's traffic, then execs dnsmasq to resolve allowed domains and populate the sets.
#
# SEE ALSO
#        dnsmasq.conf.template, healthcheck.sh, traffic.sh, o3s.json

set -euo pipefail

# Fixed image contract for the policy mount, the upstream resolver and the generated config.
CONFIG_FILE=/config/o3s.json
UPSTREAM_DNS=127.0.0.11
CONF=/etc/dnsmasq.conf

# Local port the proxy listens on
PROXY_PORT=8443

# Expiry for a resolved entry (seconds), refreshed on each resolution. Static seeds never expire.
IPSET_TIMEOUT=3600

# Filled by parse_allowlist and read by everything after it.
PORTS=""                    # distinct ports across the whole file
declare -a STATIC_SEEDS=()  # "port address" for each IPv4/CIDR entry
declare -a DOMAIN_LINES=()  # resolver config lines, an upstream and a set line per domain
# Set names per bare domain, unioned so an apex and its wildcard entry share one resolver record.
declare -A DOMAIN_SETS=()
declare -a HOST_NAMES=()    # every allowed host, in policy order, its counting set named by its index
declare -a HOST_STATIC=()   # "set address" for each IPv4/CIDR host, seeded into its counting set

# The connection mark of the gateway's own probes, kept out of every host's traffic.
PROBE_MARK=0x1
# The user the health check probes as, so its connections carry that mark.
PROBE_USER=nobody
# Where the resolver logs the names the cage asks for.
DNS_LOG_DIR=/var/log/o3s

log() { echo "[gateway] INFO: $*"; }
die() { echo "[gateway] ERROR: $*" >&2; exit 1; }

# An address is static when it is an IPv4 host or CIDR, otherwise a domain.
is_ipv4() { [[ "$1" =~ ^[0-9]+(\.[0-9]+){3}(/[0-9]+)?$ ]]; }
is_domain() { [[ "$1" =~ ^[a-zA-Z0-9]([a-zA-Z0-9.-]*[a-zA-Z0-9])?$ ]]; }
# A wildcard "*." declares a name's subdomains, and the resolver works on the bare domain either way.
bare_domain() { printf '%s' "${1#\*.}"; }

# First (link-scope) subnet on an interface, e.g. 10.10.0.0/24
link_subnet() {
  ip route show dev "$1" scope link | awk '{print $1; exit}'
}

# Deny forwarding before the route and NAT come up, so the cage is denied from the first instruction.
fail_closed() {
  iptables -P FORWARD DROP
  iptables -P INPUT DROP
  ip6tables -P FORWARD DROP 2>/dev/null || true
}

# Key off GW_CAGE_IP, the one hint compose gives, since attach order and routes both mislead here.
locate_interfaces() {
  [ -n "${GW_CAGE_IP:-}" ] || die "GW_CAGE_IP not set (expected from docker-compose environment)"

  # The cage interface holds the known cage IP, and egress is the other non-loopback interface
  ADDRS="$(ip -o -4 addr show)"
  CAGE_IF="$(echo "$ADDRS" | awk -v ip="$GW_CAGE_IP" '{split($4,a,"/"); if (a[1]==ip) {print $2; exit}}')"
  [ -n "$CAGE_IF" ] || die "no interface holds cage IP $GW_CAGE_IP"

  CAGE_SUBNET="$(link_subnet "$CAGE_IF")"
  [ -n "$CAGE_SUBNET" ] || die "cannot determine cage subnet on $CAGE_IF"

  EGRESS_IF="$(echo "$ADDRS" | awk -v c="$CAGE_IF" '$2 != "lo" && $2 != c {print $2; exit}')"
  [ -n "$EGRESS_IF" ] || die "cannot find egress interface (is the gateway attached to the egress network?)"

  # The gateway's own default route must exit via egress, at the .1 of that subnet
  EGRESS_NET="$(link_subnet "$EGRESS_IF")"
  [ -n "$EGRESS_NET" ] || die "cannot determine egress subnet on $EGRESS_IF"
  EGRESS_GW="${EGRESS_NET%/*}"; EGRESS_GW="${EGRESS_GW%.*}.1"
  ip route replace default via "$EGRESS_GW" dev "$EGRESS_IF"

  log "egress=$EGRESS_IF (gw $EGRESS_GW)  cage=$CAGE_IF ($CAGE_SUBNET, gw $GW_CAGE_IP)"
}

# One set per distinct port, with static addresses seeding theirs directly and domains on resolve.
parse_allowlist() {
  # Read each host's ports and optional secret, failing on malformed config
  local config_tsv
  config_tsv="$(yq -p=json -o=tsv '[ .hosts[] | [.name, (.ports | join(" ")), (.secret // "")] ]' "$CONFIG_FILE")" \
    || die "config file $CONFIG_FILE is not valid JSON"

  # Validate each entry and sort it into a static seed or a domain's set list
  local count_set
  # Read every host of the policy
  while IFS=$'\t' read -r addr ports secret; do
    [ -n "$addr" ] || continue
    [ -n "$ports" ] || die "host $addr has no port"
    case "$addr" in *:*) die "IPv6 address $addr unsupported (gateway is IPv4-only)";; esac
    domain="$(bare_domain "$addr")"
    is_ipv4 "$addr" || is_domain "$domain" \
      || die "invalid address $addr (a wildcard is written as *.example.com)"
    # Injection identifies the host by SNI, so an inject-host must be a domain
    if [ -n "$secret" ] && is_ipv4 "$addr"; then
      die "inject-host $addr needs a hostname not an IP"
    fi

    # Each port contributes its own set, joined into the entry's domain
    for p in $ports; do
      case "$p" in ''|*[!0-9]*) die "invalid port $p for $addr";; esac
      { [ "$p" -ge 1 ] && [ "$p" -le 65535 ]; } || die "port $p out of range for $addr"
      case " $PORTS " in *" $p "*) ;; *) PORTS="$PORTS $p";; esac
      # Seed an address host directly, and add a domain's set to the line that fills it on resolve
      if is_ipv4 "$addr"; then
        STATIC_SEEDS+=("$p $addr")
      else
        case ",${DOMAIN_SETS[$domain]:-}," in
          *",allowed-p$p,"*) ;;
          *) DOMAIN_SETS[$domain]="${DOMAIN_SETS[$domain]:+${DOMAIN_SETS[$domain]},}allowed-p$p" ;;
        esac
      fi
    done

    # Each host gets a set of its own, which counts its traffic apart from every other host
    count_set="o3s-h${#HOST_NAMES[@]}"
    HOST_NAMES+=("$addr")
    # Seed an address host into its counting set, and fill a domain's on resolve
    if is_ipv4 "$addr"; then
      HOST_STATIC+=("$count_set $addr")
    else
      DOMAIN_SETS[$domain]="${DOMAIN_SETS[$domain]},$count_set"
    fi
  done <<< "$config_tsv"

  # Each domain gets its own upstream, plus the set line capturing what it resolves to
  for domain in "${!DOMAIN_SETS[@]}"; do
    DOMAIN_LINES+=("server=/$domain/$UPSTREAM_DNS")
    DOMAIN_LINES+=("ipset=/$domain/${DOMAIN_SETS[$domain]}")
  done
}

# One set per port, holding resolved hosts and static subnets alike so either matches a packet.
create_sets() {
  # Create the set of every port
  for p in $PORTS; do
    ipset create "allowed-p$p" hash:net timeout "$IPSET_TIMEOUT" -exist
  done

  # Addresses named directly, which carry no name to check and so keep the address-only path
  ipset create static-hosts hash:net timeout 0 -exist

  # One counting set per host, filled the way its allowing sets are
  for i in "${!HOST_NAMES[@]}"; do
    ipset create "o3s-h$i" hash:net timeout "$IPSET_TIMEOUT" -exist
  done
}

# Forwarding is set from outside this container, so verify it rather than write it.
require_forwarding() {
  [ "$(cat /proc/sys/net/ipv4/ip_forward)" = 1 ] || die "net.ipv4.ip_forward is not enabled (set it via docker-compose sysctls)"
}

# Turn the parsed sets into NAT, INPUT and FORWARD rules.
apply_firewall() {
  # NAT the cage out, added idempotently so the rules already in this table survive
  iptables -t nat -C POSTROUTING -s "$CAGE_SUBNET" -o "$EGRESS_IF" -j MASQUERADE 2>/dev/null \
    || iptables -t nat -A POSTROUTING -s "$CAGE_SUBNET" -o "$EGRESS_IF" -j MASQUERADE

  # Divert cage HTTPS to the proxy, which decides the name once the address is already allowed
  if ipset list -n allowed-p443 >/dev/null 2>&1; then
    iptables -t nat -C PREROUTING -s "$CAGE_SUBNET" -p tcp --dport 443 -m set --match-set allowed-p443 dst -m set ! --match-set static-hosts dst -j REDIRECT --to-ports "$PROXY_PORT" 2>/dev/null \
      || iptables -t nat -A PREROUTING -s "$CAGE_SUBNET" -p tcp --dport 443 -m set --match-set allowed-p443 dst -m set ! --match-set static-hosts dst -j REDIRECT --to-ports "$PROXY_PORT"
  fi

  # Clamp MSS to path MTU on SYN to avoid stalls through nested NAT
  iptables -t mangle -F FORWARD
  iptables -t mangle -A FORWARD -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu

  # Count every host's traffic both ways, leaving out the gateway's own probes
  iptables -N O3S_ACCT 2>/dev/null || iptables -F O3S_ACCT
  # Count every host
  for i in "${!HOST_NAMES[@]}"; do
    # Count both directions of the host's traffic
    for way in dst src; do
      iptables -A O3S_ACCT -m connmark ! --mark "$PROBE_MARK" -m set --match-set "o3s-h$i" "$way" -m comment --comment "${HOST_NAMES[$i]}"
    done
  done

  # Mark the probes the health check opens, then count what the gateway itself sends, the proxy's connections included
  iptables -F OUTPUT
  iptables -A OUTPUT -m conntrack --ctstate NEW -m owner --uid-owner "$PROBE_USER" -j CONNMARK --set-mark "$PROBE_MARK"
  iptables -A OUTPUT -j O3S_ACCT

  # Only cage DNS, loopback and return traffic reach the gateway itself
  iptables -F INPUT
  iptables -A INPUT -j O3S_ACCT
  iptables -A INPUT -i lo -j ACCEPT
  iptables -A INPUT -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
  iptables -A INPUT -s "$CAGE_SUBNET" -p udp --dport 53 -j ACCEPT
  iptables -A INPUT -s "$CAGE_SUBNET" -p tcp --dport 53 -j ACCEPT
  # A diverted packet arrives locally, so admit it here
  iptables -A INPUT -s "$CAGE_SUBNET" -p tcp --dport "$PROXY_PORT" -j ACCEPT
  iptables -A INPUT -p icmp -j ACCEPT

  # Default-deny, so only allow-listed destinations leave the cage on their declared ports
  iptables -F FORWARD
  iptables -A FORWARD -j O3S_ACCT
  iptables -A FORWARD -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
  # One rule per port, pairing it with the set of addresses allowed on it
  for p in $PORTS; do
    iptables -A FORWARD -s "$CAGE_SUBNET" -p tcp -m set --match-set "allowed-p$p" dst --dport "$p" -j ACCEPT
  done
  # Deny cage QUIC/HTTP-3, since interception is TCP-only and UDP would bypass the proxy
  iptables -A FORWARD -s "$CAGE_SUBNET" -p udp -m multiport --dports 80,443 -j REJECT --reject-with icmp-port-unreachable
  # Fail fast instead of hanging on blocked connections
  iptables -A FORWARD -j REJECT --reject-with icmp-admin-prohibited
}

# Seed static entries directly, since no resolution will ever fill them.
seed_static() {
  # Seed only where the policy names addresses
  if [ ${#STATIC_SEEDS[@]} -gt 0 ]; then
    # Seed every address on its port
    for seed in "${STATIC_SEEDS[@]}"; do
      read -r sp saddr <<<"$seed"
      # A zero timeout keeps the entry permanently, since static seeds are added once
      ipset add -exist "allowed-p$sp" "$saddr" timeout 0
      ipset add -exist static-hosts "$saddr" timeout 0
    done
  fi

  # Seed each address host into its counting set the same way
  if [ ${#HOST_STATIC[@]} -gt 0 ]; then
    # Seed every address host into its counting set
    for seed in "${HOST_STATIC[@]}"; do
      read -r set saddr <<<"$seed"
      ipset add -exist "$set" "$saddr" timeout 0
    done
  fi
}

# Base template plus the generated lines, which point each domain at its upstream and its sets.
write_resolver_conf() {
  sed -e "s#__GW_CAGE_IP__#${GW_CAGE_IP}#g" \
      -e "s#__DNS_LOG_DIR__#${DNS_LOG_DIR}#g" \
      /etc/dnsmasq.conf.template > "$CONF"

  # The resolver writes its query log as its own user, so the folder is its own
  install -d -o dnsmasq -m 0755 "$DNS_LOG_DIR"
  # Start every lifetime with an empty log, since the record already holds what earlier ones refused
  rm -f "$DNS_LOG_DIR"/dns.log*

  # Append the lines only where the policy names domains
  if [ ${#DOMAIN_LINES[@]} -gt 0 ]; then
    printf '%s\n' "${DOMAIN_LINES[@]}" >> "$CONF"
  fi
}

[ -f "$CONFIG_FILE" ] || die "config file $CONFIG_FILE not found"

fail_closed
locate_interfaces
parse_allowlist
create_sets
require_forwarding
apply_firewall
seed_static
write_resolver_conf

log "loaded ${#DOMAIN_SETS[@]} domain(s) + ${#STATIC_SEEDS[@]} static entry(ies) on port(s)${PORTS}"
log "starting dnsmasq on ${GW_CAGE_IP} port 53 (upstream ${UPSTREAM_DNS})"

# Record the traffic beside the resolver for the gateway's lifetime
traffic.sh &

# Run the resolver as PID 1
exec dnsmasq --keep-in-foreground --conf-file="$CONF"
