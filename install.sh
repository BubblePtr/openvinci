#!/bin/sh
# install.sh — put a vinci binary on PATH from GitHub Releases.
# Agents: curl -fsSL https://raw.githubusercontent.com/BubblePtr/openvinci/main/install.sh | sh
set -eu

REPO="BubblePtr/openvinci"
DEFAULT_DIR="${HOME}/.local/bin"

usage() {
	cat <<'EOF'
install.sh — install vinci from GitHub Releases

Usage:
  curl -fsSL https://raw.githubusercontent.com/BubblePtr/openvinci/main/install.sh | sh
  curl -fsSL ... | sh -s -- --version v0.1.0 --dir ~/.local/bin

Flags:
  --version <tag>   release tag (default: latest)
  --dir <path>      install directory (default: ~/.local/bin)
  --print-target    print the archive target for this OS/arch and exit
  -h, --help        show this help
EOF
}

die() {
	printf 'vinci-install: %s\n' "$*" >&2
	exit 1
}

target_from_uname() {
	os=$(uname -s)
	arch=$(uname -m)
	case "$os" in
	Darwin)
		case "$arch" in
		arm64) echo darwin_arm64 ;;
		*) die "unsupported macOS architecture $arch (want arm64)" ;;
		esac
		;;
	Linux)
		case "$arch" in
		x86_64 | amd64) echo linux_amd64 ;;
		aarch64 | arm64) echo linux_arm64 ;;
		*) die "unsupported Linux architecture $arch (want x86_64 or arm64)" ;;
		esac
		;;
	*)
		die "unsupported OS $os (want macOS arm64 or Linux x64/arm64)"
		;;
	esac
}

need_cmd() {
	command -v "$1" >/dev/null 2>&1 || die "need $1 on PATH"
}

file_sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		die "need sha256sum or shasum to verify the download"
	fi
}

latest_tag() {
	need_cmd curl
	body=$(curl -fsSL -A "openvinci-install" "https://api.github.com/repos/${REPO}/releases/latest") ||
		die "cannot fetch the latest GitHub release"
	tag=$(printf '%s\n' "$body" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1)
	[ -n "$tag" ] || die "no GitHub release yet; tag v0.1.0 after the release workflow exists"
	printf '%s\n' "$tag"
}

expected_sha() {
	sums=$1
	name=$2
	awk -v f="$name" '
		$2 == f || $2 == ("*" f) { print $1; found=1; exit }
		END { if (!found) exit 1 }
	' "$sums"
}

install_vinci() {
	version=$1
	dir=$2
	target=$(target_from_uname)
	archive="vinci_${target}.tar.gz"
	need_cmd curl
	need_cmd tar
	need_cmd mkdir
	need_cmd install

	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT INT HUP TERM

	base="https://github.com/${REPO}/releases/download/${version}"
	curl -fsSL -A "openvinci-install" -o "$tmp/$archive" "$base/$archive" ||
		die "cannot download $archive from $version"
	curl -fsSL -A "openvinci-install" -o "$tmp/checksums.txt" "$base/checksums.txt" ||
		die "cannot download checksums.txt from $version"

	want=$(expected_sha "$tmp/checksums.txt" "$archive") ||
		die "checksums.txt has no entry for $archive"
	got=$(file_sha256 "$tmp/$archive")
	[ "$got" = "$want" ] || die "checksum mismatch for $archive"

	tar -xzf "$tmp/$archive" -C "$tmp" vinci ||
		die "archive $archive does not contain a vinci binary"
	[ -f "$tmp/vinci" ] || die "extracted archive is missing vinci"

	mkdir -p "$dir"
	install -m 0755 "$tmp/vinci" "$dir/vinci"
	if command -v xattr >/dev/null 2>&1; then
		xattr -d com.apple.quarantine "$dir/vinci" 2>/dev/null || true
	fi

	printf 'installed %s to %s\n' "$("$dir/vinci" --version 2>/dev/null || echo vinci)" "$dir/vinci"
	case ":$PATH:" in
	*":$dir:"*) ;;
	*)
		printf 'vinci-install: add %s to PATH, e.g. export PATH="%s:$PATH"\n' "$dir" "$dir" >&2
		;;
	esac
}

version=""
dir="${VINCI_INSTALL_DIR:-$DEFAULT_DIR}"
print_target=0

while [ $# -gt 0 ]; do
	case "$1" in
	-h | --help)
		usage
		exit 0
		;;
	--print-target)
		print_target=1
		;;
	--version)
		[ $# -ge 2 ] || die "--version needs a tag"
		version=$2
		shift
		;;
	--dir)
		[ $# -ge 2 ] || die "--dir needs a path"
		dir=$2
		shift
		;;
	--version=*)
		version=${1#--version=}
		;;
	--dir=*)
		dir=${1#--dir=}
		;;
	*)
		die "unknown argument $1 (see --help)"
		;;
	esac
	shift
done

if [ -n "${VINCI_VERSION:-}" ] && [ -z "$version" ]; then
	version=$VINCI_VERSION
fi

if [ "$print_target" -eq 1 ]; then
	target_from_uname
	exit 0
fi

if [ -z "$version" ]; then
	version=$(latest_tag)
fi

install_vinci "$version" "$dir"
