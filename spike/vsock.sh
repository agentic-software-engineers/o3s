#!/bin/bash
# Spike V, run on the runner: a cage VM with no network device, whose one link is vsock to an allow-list proxy on the host
set -uo pipefail
. "$(dirname "$0")/lib.sh"

SPIKE="$(cd "$(dirname "$0")" && pwd)"
WORK="${RUNNER_TEMP:-/tmp}/vsock"
CID=3
IMAGE_URL="$(awk '/location:.*amd64/ {gsub(/"/, "", $3); print $3}' internal/vm/o3s.yaml)"
IMAGE_SUM="$(awk '/amd64/ {f=1} f && /digest:/ {gsub(/"|sha256:/, "", $2); print $2; exit}' internal/vm/o3s.yaml)"
mkdir -p "${WORK}"

# Ssh into the cage VM over vsock alone
cage() {
  ssh -i "${WORK}/key" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o ConnectTimeout=10 \
    -o ProxyCommand="python3 ${SPIKE}/vsock-relay.py stdio ${CID} 2222" ubuntu@cage "$@"
}

# The pinned image, checked against the digest the home's VM template names
check "the cage image downloads and matches its digest" bash -c "curl -fsSLo '${WORK}/base.img' '${IMAGE_URL}' && echo '${IMAGE_SUM}  ${WORK}/base.img' | sha256sum -c"
qemu-img create -q -f qcow2 -b "${WORK}/base.img" -F qcow2 "${WORK}/disk.qcow2" 30G
ssh-keygen -q -t ed25519 -N '' -f "${WORK}/key"

# The allow-list proxy on the host: names outside the list are refused, HTTPS only on 443
cat > "${WORK}/filter" <<'FILTER'
(^|\.)github\.com$
(^|\.)githubusercontent\.com$
(^|\.)ubuntu\.com$
(^|\.)docker\.io$
(^|\.)docker\.com$
(^|\.)cloudflarestorage\.com$
(^|\.)amazonaws\.com$
FILTER
cat > "${WORK}/tinyproxy.conf" <<CONF
Port 3128
Listen 127.0.0.1
Timeout 600
LogLevel Notice
LogFile "${WORK}/tinyproxy.log"
PidFile "${WORK}/tinyproxy.pid"
MaxClients 200
Filter "${WORK}/filter"
FilterType ere
FilterDefaultDeny Yes
ConnectPort 443
CONF
tinyproxy -v
tinyproxy -c "${WORK}/tinyproxy.conf"
# Carry the cage VM's vsock connections on port 3128 to the proxy
python3 "${SPIKE}/vsock-relay.py" host 3128 3128 &

# The seed: the dev user with the key, the guest relay, ssh on vsock, and the proxy for apt, the shell and Docker
relay_b64="$(base64 -w0 "${SPIKE}/vsock-relay.py")"
cat > "${WORK}/user-data" <<SEED
#cloud-config
users:
  - name: ubuntu
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    ssh_authorized_keys: ["$(cat "${WORK}/key.pub")"]
write_files:
  - path: /usr/local/libexec/vsock-relay.py
    encoding: b64
    content: ${relay_b64}
    permissions: '0755'
  - path: /etc/systemd/system/vsock-relay.service
    content: |
      [Unit]
      Description=Carry TCP 3128 to the host over vsock
      [Service]
      ExecStart=/usr/bin/python3 /usr/local/libexec/vsock-relay.py guest 3128
      Restart=always
      [Install]
      WantedBy=multi-user.target
  - path: /etc/systemd/system/vsock-ssh.socket
    content: |
      [Socket]
      ListenStream=vsock::2222
      Accept=yes
      [Install]
      WantedBy=sockets.target
  - path: /etc/systemd/system/vsock-ssh@.service
    content: |
      [Service]
      ExecStart=-/usr/sbin/sshd -i
      StandardInput=socket
      RuntimeDirectory=sshd
  - path: /etc/apt/apt.conf.d/90proxy
    content: |
      Acquire::http::Proxy "http://127.0.0.1:3128";
      Acquire::https::Proxy "http://127.0.0.1:3128";
  - path: /etc/environment
    append: true
    content: |
      http_proxy=http://127.0.0.1:3128
      https_proxy=http://127.0.0.1:3128
      HTTP_PROXY=http://127.0.0.1:3128
      HTTPS_PROXY=http://127.0.0.1:3128
      no_proxy=localhost,127.0.0.1
  - path: /etc/docker/daemon.json
    content: |
      {"proxies": {"http-proxy": "http://127.0.0.1:3128", "https-proxy": "http://127.0.0.1:3128", "no-proxy": "localhost,127.0.0.1"}}
runcmd:
  - [systemctl, daemon-reload]
  - [systemctl, enable, --now, vsock-relay.service, vsock-ssh.socket]
SEED
echo "instance-id: cage" > "${WORK}/meta-data"
cloud-localds "${WORK}/seed.img" "${WORK}/user-data" "${WORK}/meta-data"

# Boot the cage VM with no network device, only vsock
start=${SECONDS}
qemu-system-x86_64 -machine q35,accel=kvm -cpu host -smp 2 -m 6G -nic none \
  -drive file="${WORK}/disk.qcow2",if=virtio -drive file="${WORK}/seed.img",if=virtio,format=raw \
  -device vhost-vsock-pci,guest-cid=${CID} -display none -serial file:"${WORK}/console.log" -daemonize
# Wait up to 15 minutes for ssh to answer over vsock
answers() {
  local until=$((SECONDS + 900))
  # Try again every few seconds until it answers or the time runs out
  until cage true 2>/dev/null; do
    [ "${SECONDS}" -lt "${until}" ] || return 1
    sleep 5
  done
}
gate "the cage VM answers ssh over vsock" answers
echo "RESULT | the cage VM boots to ssh over vsock | INFO | $((SECONDS - start))s |"

# Succeed when the cage VM lists loopback alone among its links
loopback_alone() {
  cage ip -o link | awk -F': ' '{print $2}'
  [ -z "$(cage ip -o link | awk -F': ' '$2 != "lo" {print $2}')" ]
}
# Succeed when the cage VM holds no route at all
no_route() {
  [ -z "$(cage ip route)" ]
}
check "the cage VM holds no network device but loopback" loopback_alone
check "the cage VM holds no route" no_route
check "the cage reaches github.com through the proxy" cage curl -fsS -o /dev/null --max-time 20 https://github.com
check "the proxy refuses example.com" fails cage curl -fsS -o /dev/null --max-time 20 https://example.com
check "the cage has no path to a raw address" fails cage curl -fsS -o /dev/null --max-time 10 --noproxy '*' https://1.1.1.1
check "the cage installs Docker through the proxy" cage 'sudo apt-get update -q >/dev/null && sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -q docker.io >/dev/null && sudo usermod -aG docker ubuntu'
check "the cage runs hello-world" cage docker run --rm hello-world
check "the cage installs kind" cage 'sudo curl -fsSLo /usr/local/bin/kind https://github.com/kubernetes-sigs/kind/releases/download/v0.33.0/kind-linux-amd64 && echo "aee6151561422756b764a4ae28e7f44cda5af5a9eead3cc9985112b1de8d8e0d  /usr/local/bin/kind" | sha256sum -c && sudo chmod +x /usr/local/bin/kind'
check "the cage pulls kind's node image" cage docker pull -q kindest/node:v1.37.0@sha256:a1ed56cfb0e7b93589bdf97c8cd566405a265939e3620fc4f5de89adff580ae5
# Raise the cluster without proxy variables, which kind would hand its nodes, three times to time it
for run in 1 2 3; do
  check "the cage raises a kind cluster, run ${run}" cage 'env -u http_proxy -u https_proxy -u HTTP_PROXY -u HTTPS_PROXY kind create cluster --wait 5m'
  cage kind delete cluster >/dev/null 2>&1
done
cage free -m
echo "--- proxy log"
tail -30 "${WORK}/tinyproxy.log"
echo "--- console"
tail -40 "${WORK}/console.log"
