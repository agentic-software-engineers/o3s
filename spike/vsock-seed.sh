#!/bin/bash
# Write the cage VM's key and NoCloud seed into $1: the dev user, the guest relay, ssh on vsock, and the proxy for apt, the shell and Docker
work=$1
spike="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ssh-keygen -q -t ed25519 -N '' -f "${work}/key"
relay_b64="$(base64 < "${spike}/vsock-relay.py" | tr -d '\n')"
cat > "${work}/user-data" <<SEED
#cloud-config
users:
  - name: ubuntu
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    ssh_authorized_keys: ["$(cat "${work}/key.pub")"]
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
  - [sh, -c, "modprobe hv_sock || true"]
  - [systemctl, daemon-reload]
  - [systemctl, enable, --now, vsock-relay.service, vsock-ssh.socket]
SEED
echo "instance-id: cage" > "${work}/meta-data"
