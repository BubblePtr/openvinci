#!/bin/sh
# Offline tests for install.sh --print-target. The public install contract
# is one curl|sh line; this file only exercises platform detection.
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
install="$root/install.sh"
fail=0

if [ ! -x "$install" ] && [ ! -f "$install" ]; then
	echo "missing $install"
	exit 1
fi

stub_uname() {
	os=$1
	arch=$2
	dir=$(mktemp -d)
	cat >"$dir/uname" <<EOF
#!/bin/sh
if [ "\$1" = -s ]; then echo $os; exit 0; fi
if [ "\$1" = -m ]; then echo $arch; exit 0; fi
exit 1
EOF
	chmod +x "$dir/uname"
	echo "$dir"
}

assert_target() {
	os=$1
	arch=$2
	want=$3
	dir=$(stub_uname "$os" "$arch")
	got=$(PATH="$dir:$PATH" sh "$install" --print-target) || {
		echo "FAIL $os/$arch: install.sh exited $?"
		fail=1
		rm -rf "$dir"
		return
	}
	if [ "$got" != "$want" ]; then
		echo "FAIL $os/$arch: got '$got' want '$want'"
		fail=1
	fi
	rm -rf "$dir"
}

assert_unsupported() {
	os=$1
	arch=$2
	dir=$(stub_uname "$os" "$arch")
	if PATH="$dir:$PATH" sh "$install" --print-target >/dev/null 2>&1; then
		echo "FAIL $os/$arch: want a non-zero exit"
		fail=1
	fi
	rm -rf "$dir"
}

assert_target Darwin arm64 darwin_arm64
assert_target Linux x86_64 linux_amd64
assert_target Linux amd64 linux_amd64
assert_target Linux aarch64 linux_arm64
assert_target Linux arm64 linux_arm64
assert_unsupported Darwin x86_64
assert_unsupported Darwin amd64
assert_unsupported Linux i386
assert_unsupported Windows_NT AMD64
assert_unsupported MINGW64_NT-10.0 x86_64

if [ "$fail" -ne 0 ]; then
	echo "install.sh tests failed"
	exit 1
fi
echo "ok install.sh --print-target"
