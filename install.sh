#!/bin/sh
set -eu

repo=${AGENT2HOST_REPOSITORY:-wangzhaode/agent2host}
install_dir=${A2H_INSTALL_DIR:-"$HOME/.local/bin"}
if [ "$#" -eq 0 ]; then
  set -- all
fi

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) printf 'a2h: unsupported operating system: %s\n' "$(uname -s)" >&2; exit 1 ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  riscv64) arch=riscv64 ;;
  *) printf 'a2h: unsupported architecture: %s\n' "$(uname -m)" >&2; exit 1 ;;
esac

if [ "$os" = darwin ] && [ "$arch" = riscv64 ]; then
  printf 'a2h: darwin/riscv64 is not supported\n' >&2
  exit 1
fi
if ! command -v curl >/dev/null 2>&1; then
  printf '%s\n' 'a2h: curl is required' >&2
  exit 127
fi

archive="agent2host_${os}_${arch}.tar.gz"
base_url=${AGENT2HOST_DOWNLOAD_BASE_URL:-"https://github.com/${repo}/releases/latest/download"}
tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/agent2host.XXXXXX")
trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM

curl -fsSL "$base_url/$archive" -o "$tmp_dir/$archive"
curl -fsSL "$base_url/agent2host_checksums.txt" -o "$tmp_dir/checksums.txt"

expected=$(awk -v file="$archive" '$2 == file { print $1 }' "$tmp_dir/checksums.txt")
if [ -z "$expected" ]; then
  printf 'a2h: checksum not found for %s\n' "$archive" >&2
  exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$tmp_dir/$archive" | awk '{ print $1 }')
elif command -v shasum >/dev/null 2>&1; then
  actual=$(shasum -a 256 "$tmp_dir/$archive" | awk '{ print $1 }')
else
  printf '%s\n' 'a2h: sha256sum or shasum is required' >&2
  exit 127
fi
if [ "$actual" != "$expected" ]; then
  printf 'a2h: checksum mismatch for %s\n' "$archive" >&2
  exit 1
fi

tar -xzf "$tmp_dir/$archive" -C "$tmp_dir"
mkdir -p "$install_dir"
if [ -x "$install_dir/a2h" ]; then
  "$install_dir/a2h" daemon stop >/dev/null 2>&1 || true
fi
install -m 0755 "$tmp_dir/a2h" "$install_dir/a2h"
"$install_dir/a2h" setup "$@"

printf '\nAgent2Host installed at %s\n' "$install_dir/a2h"
case ":$PATH:" in
  *":$install_dir:"*) ;;
  *) printf 'Add %s to PATH to run a2h directly.\n' "$install_dir" ;;
esac
