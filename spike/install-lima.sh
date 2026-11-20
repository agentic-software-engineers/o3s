#!/bin/bash
# Install the pinned Lima release into /usr/local, owned by root, checked against its checksum
set -euo pipefail
version=2.3.0
sum=cc8ba97f1c801b899f7c9eda9919dfc4ce4fa2c140c53e5add15aa2d0a1936f9
archive="/tmp/lima-${version}-Linux-x86_64.tar.gz"
curl -fsSLo "${archive}" "https://github.com/lima-vm/lima/releases/download/v${version}/lima-${version}-Linux-x86_64.tar.gz"
echo "${sum}  ${archive}" | sha256sum -c
sudo tar --no-same-owner -C /usr/local -xzf "${archive}"
limactl --version
