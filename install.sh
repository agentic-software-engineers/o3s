#!/bin/sh
# NAME
#        install.sh — installs the o3s binary, the devcontainer CLI it rebuilds the cage with, and Lima, which runs the home's VM
#
# SYNOPSIS
#        curl -fsSL https://github.com/hansehart/o3s/releases/latest/download/install.sh | sh
#        sh install.sh [--rc | --version <X.Y.Z[-rc.N]>] [--no-devcontainer] [--no-lima]
#
# DESCRIPTION
#        Downloads the o3s binary of a release for this host, checks it against the release's
#        checksums, installs it to ~/.local/bin under its channel's name, and names the command
#        that verifies its build attestation by hand. The latest stable release goes to
#        o3s-latest, which o3s links to. --rc takes the newest release, release candidates
#        included, as o3s-rc, and --version a given release as o3s-<version>. Each updates
#        within its channel. Then installs the devcontainer CLI o3s rebuilds the cage with, with
#        its own Node, into ~/.local/share/o3s/devcontainers. Then installs Lima, which runs the
#        home's VM, checked against the checksum this script pins for its archive, into
#        ~/.local/share/o3s/lima, unless run with --no-lima. On Linux, Lima runs VMs with QEMU,
#        which the system's package manager installs.
#
# ENVIRONMENT
#        O3S_INSTALL_DIR             where the binary goes (default: ~/.local/bin)
#        O3S_RELEASE_URL             where the release's files come from (default: GitHub releases)
#        O3S_RELEASES_FEED           the releases feed --rc reads (default: GitHub's releases.atom)
#        O3S_LIMA_URL                where Lima's archive comes from, still checked against its pin (default: GitHub releases)
#        XDG_DATA_HOME               the data folder o3s keeps its devcontainer CLI and Lima in (default: ~/.local/share)
set -eu

repo="hansehart/o3s"
signer="${repo}/.github/workflows/publish.yaml"

# The devcontainer CLI's installer at a pinned tag, and the checksum it has there.
devcontainers_version="0.90.0"
devcontainers_installer_sha256="137e273aea49666a6ed9b5b0796153efc5873d8778f69ce552beb6ff3fab2d69"

# Lima at a pinned release, and the checksum of its archive for each system and architecture, from the release's SHA256SUMS.
lima_version="2.3.0"
lima_sha256_linux_amd64="cc8ba97f1c801b899f7c9eda9919dfc4ce4fa2c140c53e5add15aa2d0a1936f9"
lima_sha256_linux_arm64="7e295d243d9b9439fa05628500daff4371a6861cc7e995ac4af4a0781f736cc6"
lima_sha256_darwin_amd64="d58bc7d6b8158d638720d8c8fdb17eb58527dfead6a29a5b60f48a35ee4a65a6"
lima_sha256_darwin_arm64="b7af444bd8f91d3c307477e562f2cf3bd7595d62b9429185e4fa841563578cfc"

# say prints a line for the user.
say() {
	printf '%s\n' "$*"
}

# die names why the install stops, and stops it.
die() {
	printf 'install.sh: %s\n' "$*" >&2
	exit 1
}

# fetch downloads a URL into a file, over HTTPS alone for the default addresses.
fetch() {
	# The protocol flags split into words on purpose
	# shellcheck disable=SC2086
	curl -fsSL ${proto} -o "$2" "$1"
}

# check_sums checks every file a checksum list names in the folder that holds it, with the checksum tool the host carries.
check_sums() {
	# Check with sha256sum where the host has it, and with macOS's shasum otherwise
	if command -v sha256sum >/dev/null 2>&1; then
		(cd "$1" && sha256sum -c "$2" >/dev/null 2>&1)
	else
		(cd "$1" && shasum -a 256 -c "$2" >/dev/null 2>&1)
	fi
}

version=""
channel="latest"
devcontainer="yes"
lima="yes"
# Read every option
while [ $# -gt 0 ]; do
	# Take each option the script knows, and name any other
	case "$1" in
	--version)
		[ $# -ge 2 ] || die "--version needs a version, as in --version 1.0.0 or --version 1.0.0-rc.1"
		version="${2#v}"
		channel="pinned"
		shift 2
		;;
	--rc)
		channel="rc"
		shift
		;;
	--no-devcontainer)
		devcontainer="no"
		shift
		;;
	--no-lima)
		lima="no"
		shift
		;;
	--preview)
		# Take the flag of the VM's preview as it stands, the VM being the default now
		shift
		;;
	-h | --help)
		say "Usage: sh install.sh [--rc | --version <X.Y.Z[-rc.N]>] [--no-devcontainer] [--no-lima]"
		exit 0
		;;
	*)
		die "unknown option $1, see --help"
		;;
	esac
done

kernel=$(uname -s)
arch=$(uname -m)
# Accept the kernels o3s ships for
case "${kernel}" in
Linux | Darwin) ;;
*) die "o3s runs on Linux and macOS, and this host runs ${kernel}" ;;
esac
# Name each architecture by the name the release gives it
case "${arch}" in
x86_64 | amd64) arch="amd64" ;;
aarch64 | arm64) arch="arm64" ;;
*) die "o3s ships for amd64 and arm64, and this host is ${arch}" ;;
esac
os=$(printf '%s' "${kernel}" | tr '[:upper:]' '[:lower:]')
asset="o3s-${os}-${arch}"
# Name Lima's archive and its pinned checksum for this host, by the names Lima's release gives them
case "${os}-${arch}" in
linux-amd64) lima_platform="Linux-x86_64" lima_sha256="${lima_sha256_linux_amd64}" ;;
linux-arm64) lima_platform="Linux-aarch64" lima_sha256="${lima_sha256_linux_arm64}" ;;
darwin-amd64) lima_platform="Darwin-x86_64" lima_sha256="${lima_sha256_darwin_amd64}" ;;
darwin-arm64) lima_platform="Darwin-arm64" lima_sha256="${lima_sha256_darwin_arm64}" ;;
esac

data="${HOME}/.local/share"
# Follow the data folder the environment names, where it is absolute as the XDG spec asks
case "${XDG_DATA_HOME:-}" in
/*) data="${XDG_DATA_HOME}" ;;
esac
prefix="${data}/o3s/devcontainers"
lima_prefix="${data}/o3s/lima"
# Name the tool Lima's archive unpacks with, before anything installs
if [ "${lima}" = "yes" ] && [ ! -x "${lima_prefix}/bin/limactl" ]; then
	command -v tar >/dev/null 2>&1 || die "Lima's archive unpacks with tar, so install it, or run install.sh with --no-lima"
fi
# Name the tools the devcontainer CLI's installer unpacks Node with, before anything installs
if [ "${devcontainer}" = "yes" ] && [ ! -x "${prefix}/bin/devcontainer" ]; then
	# Check each tool in turn
	for tool in tar xz; do
		command -v "${tool}" >/dev/null 2>&1 ||
			die "the devcontainer CLI's installer needs tar and xz to unpack Node, so install them, or run install.sh with --no-devcontainer"
	done
fi

proto="--proto =https --tlsv1.2"
tmp=$(mktemp -d)
trap 'rm -rf "${tmp}"' EXIT
# Take the newest release from the releases feed for the release candidates' channel
if [ "${channel}" = "rc" ]; then
	feed="https://github.com/${repo}/releases.atom"
	# Read the feed from where the environment points, a mirror or a test, over any protocol
	if [ -n "${O3S_RELEASES_FEED:-}" ]; then
		feed="${O3S_RELEASES_FEED}"
		proto=""
	fi
	fetch "${feed}" "${tmp}/releases.atom" || die "failed to read the releases feed ${feed}"
	version=$(grep -o 'releases/tag/v[^"]*' "${tmp}/releases.atom" | head -n 1 | sed 's|.*/tag/v||')
	[ -n "${version}" ] || die "the releases feed ${feed} names no release"
	say "Taking the newest release, ${version}"
	proto="--proto =https --tlsv1.2"
fi
base="https://github.com/${repo}/releases/latest/download"
# Take the release the user named
if [ -n "${version}" ]; then
	base="https://github.com/${repo}/releases/download/v${version}"
fi
# Take the files from where the environment points, a mirror or a test, over any protocol
if [ -n "${O3S_RELEASE_URL:-}" ]; then
	base="${O3S_RELEASE_URL}"
	proto=""
fi

fetch "${base}/${asset}" "${tmp}/${asset}" || die "failed to download ${asset} from ${base}"
fetch "${base}/checksums.txt" "${tmp}/checksums.txt" || die "failed to download checksums.txt from ${base}"
grep "  ${asset}\$" "${tmp}/checksums.txt" >"${tmp}/${asset}.sha256" || die "checksums.txt names no ${asset}"
check_sums "${tmp}" "${asset}.sha256" || die "${asset} differs from its line in checksums.txt, so the download stays uninstalled"

dir="${O3S_INSTALL_DIR:-${HOME}/.local/bin}"
mkdir -p "${dir}"
name="o3s-${channel}"
# Name a pinned binary after the release it keeps
if [ "${channel}" = "pinned" ]; then
	name="o3s-${version}"
fi
install -m 0755 "${tmp}/${asset}" "${dir}/${name}"
# Link o3s to the stable channel's binary
if [ "${channel}" = "latest" ]; then
	ln -sfn "${name}" "${dir}/o3s"
fi
say "Installed $("${dir}/${name}" --version) to ${dir}/${name}"
say "Verify its build attestation with: gh attestation verify ${dir}/${name} --repo ${repo} --signer-workflow ${signer}"
# Show how to reach the binary from a folder outside the PATH
case ":${PATH}:" in
*":${dir}:"*) ;;
*) say "Add ${dir} to your PATH: export PATH=\"${dir}:\$PATH\"" ;;
esac

# Install the devcontainer CLI, which o3s rebuilds the cage with
if [ "${devcontainer}" = "yes" ]; then
	# Keep a devcontainer CLI the host holds already
	if [ -x "${prefix}/bin/devcontainer" ]; then
		say "Found the devcontainer CLI in ${prefix}"
	else
		proto="--proto =https --tlsv1.2"
		fetch "https://raw.githubusercontent.com/devcontainers/cli/v${devcontainers_version}/scripts/install.sh" "${tmp}/devcontainers.sh" ||
			die "failed to download the devcontainer CLI's installer"
		printf '%s  devcontainers.sh\n' "${devcontainers_installer_sha256}" >"${tmp}/devcontainers.sha256"
		check_sums "${tmp}" "devcontainers.sha256" || die "the devcontainer CLI's installer differs from its pinned checksum"
		# Make the folder above the CLI's, which its installer asks to be writable
		mkdir -p "$(dirname "${prefix}")" || die "failed to create $(dirname "${prefix}")"
		sh "${tmp}/devcontainers.sh" --prefix "${prefix}" --version "${devcontainers_version}"
	fi
fi

# Install Lima, which runs the home's VM
if [ "${lima}" = "yes" ]; then
	# Keep a Lima the host holds already
	if [ -x "${lima_prefix}/bin/limactl" ]; then
		say "Found Lima in ${lima_prefix}"
	else
		proto="--proto =https --tlsv1.2"
		lima_base="https://github.com/lima-vm/lima/releases/download/v${lima_version}"
		# Take the archive from where the environment points, a mirror or a test, over any protocol, still held to its pin
		if [ -n "${O3S_LIMA_URL:-}" ]; then
			lima_base="${O3S_LIMA_URL}"
			proto=""
		fi
		archive="lima-${lima_version}-${lima_platform}.tar.gz"
		fetch "${lima_base}/${archive}" "${tmp}/${archive}" || die "failed to download ${archive} from ${lima_base}"
		printf '%s  %s\n' "${lima_sha256}" "${archive}" >"${tmp}/lima.sha256"
		check_sums "${tmp}" "lima.sha256" || die "Lima's archive ${archive} differs from its pinned checksum, so it stays unpacked"
		mkdir -p "${lima_prefix}" || die "failed to create ${lima_prefix}"
		tar -C "${lima_prefix}" -xzf "${tmp}/${archive}" || die "failed to unpack ${archive} into ${lima_prefix}"
		say "Installed Lima ${lima_version} to ${lima_prefix}"
	fi
	# Point a Linux host without QEMU, which Lima runs VMs with there, at the packages it needs
	if [ "${os}" = "linux" ]; then
		qemu_arch="x86_64"
		# Name QEMU's binary for this architecture
		if [ "${arch}" = "arm64" ]; then
			qemu_arch="aarch64"
		fi
		# Name the packages where either QEMU tool is missing
		if ! command -v "qemu-system-${qemu_arch}" >/dev/null 2>&1 || ! command -v qemu-img >/dev/null 2>&1; then
			# Give the command known for amd64, and the tools to look for otherwise
			if [ "${arch}" = "amd64" ]; then
				say "Install QEMU, which Lima runs VMs with on Linux, as in: sudo apt-get install qemu-system-x86 qemu-utils ovmf"
			else
				say "Install QEMU, which Lima runs VMs with on Linux, so that qemu-system-${qemu_arch} and qemu-img are on PATH"
			fi
		fi
	fi
fi

say ""
command="${name}"
# Call the stable channel by its link
if [ "${channel}" = "latest" ]; then
	command="o3s"
fi
say "Next: mkdir ~/o3s && cd ~/o3s && ${command} init"
