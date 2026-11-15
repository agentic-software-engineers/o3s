#!/bin/sh
# Proves inside a VM what the o3s stack needs there: AppArmor on, and Docker confining containers with it
set -eux

# Name the kernel and release the evidence comes from
uname -srm
. /etc/os-release
echo "${PRETTY_NAME}"

# Mount securityfs where the init left it out, as WSL does (o3s wiki, Compatibility)
if ! mountpoint -q /sys/kernel/security; then
  mount -t securityfs none /sys/kernel/security
fi

# Require AppArmor in the kernel's active LSM list
cat /sys/kernel/security/lsm
test "$(cat /sys/module/apparmor/parameters/enabled)" = Y

# Install Docker from the release's own archive
apt-get update -q
DEBIAN_FRONTEND=noninteractive apt-get install -yq docker.io

# Require the daemon to report AppArmor among its security options
docker info --format '{{json .SecurityOptions}}' | tee /dev/stderr | grep -q apparmor

# Require a container to run confined by Docker's default profile
confinement="$(docker run --rm ubuntu:24.04@sha256:2198e4c65076e9cb0b3d695219df667c0e3b0072da64ca3c2648aca17dcecaf5 cat /proc/self/attr/current)"
echo "${confinement}"
test "${confinement}" = "docker-default (enforce)"

# List the host folders the VM sees, for the record
mount | grep -E ' type (9p|virtiofs|fuse\.sshfs|drvfs)' || echo "no host folders mounted"
