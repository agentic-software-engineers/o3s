#!/bin/bash
# Shared steps of the two-VM spike: timed checks, a fenced Lima instance, and the allow-list

# The hosts the cage reaches: the code forge and the registry the workloads pull from
ALLOW=(github.com registry-1.docker.io auth.docker.io production.cloudflare.docker.com
  docker-images-prod.6aa30f8b08e16409b46e0173d6de2f56.r2.cloudflarestorage.com)

# Run a named check and print one result row with its outcome and duration
check() {
  local name=$1 start=$SECONDS
  shift
  # Report the outcome either way, so every check lands in the summary
  if "$@"; then
    echo "RESULT | ${name} | PASS | $((SECONDS - start))s |"
  else
    echo "RESULT | ${name} | FAIL | $((SECONDS - start))s |"
    return 1
  fi
}

# Run a named check that every later step depends on, ending the layout when it fails
gate() {
  check "$@" || exit 1
}

# Succeed when the command fails, for checks that expect a refusal
fails() {
  ! "$@"
}

# Start a Lima instance inside its own systemd scope, so the firewall tells its traffic apart by cgroup
start_fenced() {
  local name=$1
  shift
  sudo systemd-run --quiet --scope --unit="${name}" -- \
    sudo -H -u "${USER}" env PATH="${PATH}" limactl start --name="${name}" --vm-type=qemu --timeout=30m --tty=false "$@"
}

# Let the scope's processes reach loopback and the given addresses on 443 alone, and log the rest before refusing it
fence() {
  local scope=$1
  shift
  sudo iptables -N fence
  sudo iptables -A fence -o lo -j ACCEPT
  # Open each allowed address to HTTPS
  for ip in "$@"; do
    sudo iptables -A fence -d "${ip}" -p tcp --dport 443 -j ACCEPT
  done
  sudo iptables -A fence -j LOG --log-prefix "fence-refused "
  sudo iptables -A fence -j REJECT
  sudo iptables -I OUTPUT 1 -m cgroup --path "system.slice/${scope}.scope" -j fence
}

# Print the IPv4 addresses the allowed hosts resolve to here
allowed_ips() {
  getent ahostsv4 "${ALLOW[@]}" | awk '{print $1}' | sort -u
}

# Check that the instance's QEMU runs on KVM inside the instance's scope
on_kvm_in_scope() {
  local name=$1 pid
  pid=$(pgrep -f "qemu-system.*lima-${name}") || return 1
  tr '\0' ' ' < "/proc/${pid}/cmdline" | grep -q "accel=kvm" && grep -qx "${pid}" "/sys/fs/cgroup/system.slice/${name}.scope/cgroup.procs"
}

# Install kind in the instance from its pinned release, checked against its checksum
install_kind() {
  limactl shell "$1" -- sudo sh -c '
    curl -fsSLo /usr/local/bin/kind https://github.com/kubernetes-sigs/kind/releases/download/v0.33.0/kind-linux-amd64
    echo "aee6151561422756b764a4ae28e7f44cda5af5a9eead3cc9985112b1de8d8e0d  /usr/local/bin/kind" | sha256sum -c
    chmod +x /usr/local/bin/kind'
}

# Check from inside the instance that the allowed host connects and another is refused
egress_checks() {
  local name=$1
  check "the cage reaches github.com" limactl shell "${name}" -- curl -sS -o /dev/null --max-time 20 https://github.com
  check "the cage is refused example.com" fails limactl shell "${name}" -- curl -sS -o /dev/null --max-time 20 https://example.com
}

# Run the engine and cluster checks from inside the instance, once HTTPS is open to the registries' storage
workload_checks() {
  local name=$1
  check "the cage runs hello-world" limactl shell "${name}" -- docker run --rm hello-world
  check "the cage raises a kind cluster" limactl shell "${name}" -- kind create cluster --wait 5m
  check "the kind node is Ready" limactl shell "${name}" -- docker exec kind-control-plane kubectl wait --for=condition=Ready node --all --timeout=60s
}

# Print the memory in use and the refused connections the fence logged
report() {
  free -m
  limactl list
  sudo journalctl -k --no-pager | grep fence-refused | tail -20 || true
}
