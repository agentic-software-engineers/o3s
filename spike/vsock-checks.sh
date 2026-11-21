#!/bin/bash
# The checks every platform runs once its cage VM is booting: callers define cage, START and PROXY_LOG

# Wait up to 15 minutes for ssh to answer over vsock
answers() {
  local until=$((SECONDS + 900))
  # Try again every few seconds until it answers or the time runs out
  until cage true 2>/dev/null; do
    [ "${SECONDS}" -lt "${until}" ] || return 1
    sleep 5
  done
}
# Succeed when the cage VM lists loopback alone among its links
loopback_alone() {
  cage ip -o link | awk -F': ' '{print $2}'
  [ -z "$(cage ip -o link | awk -F': ' '$2 != "lo" {print $2}')" ]
}
# Succeed when the cage VM holds no route at all
no_route() {
  [ -z "$(cage ip route)" ]
}

gate "the cage VM answers ssh over vsock" answers
echo "RESULT | the cage VM boots to ssh over vsock | INFO | $((SECONDS - START))s |"
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
tail -30 "${PROXY_LOG}"
