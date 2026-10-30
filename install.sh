#!/bin/sh
# Installs the latest canape release, for machines without Homebrew:
#   curl -fsSL https://raw.githubusercontent.com/lucasnevespereira/canape/main/install.sh | sh
set -eu

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case $os in
darwin | linux) ;;
*) echo "canape runs on macOS and Linux, not $os." >&2; exit 1 ;;
esac

case $(uname -m) in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
armv7l) arch=arm ;; # 32-bit Raspberry Pi OS
*) echo "There's no canape build for $(uname -m) yet." >&2; exit 1 ;;
esac

name=canape-$os-$arch
url=https://github.com/lucasnevespereira/canape/releases/latest/download
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cd "$tmp"

echo "Downloading $name..."
curl -fsSL "$url/$name.tar.gz" -o "$name.tar.gz"
curl -fsSL "$url/SHA256SUMS" -o SHA256SUMS
grep " $name.tar.gz\$" SHA256SUMS >sum
if command -v sha256sum >/dev/null; then sha256sum -c sum >/dev/null; else shasum -a 256 -c sum >/dev/null; fi
tar -xzf "$name.tar.gz"

dir=/usr/local/bin
sudo=""
if ! { mkdir -p "$dir" 2>/dev/null && [ -w "$dir" ]; }; then
	echo "Installing into $dir needs your password."
	sudo=sudo
	sudo mkdir -p "$dir"
fi
$sudo install -m 0755 "$name" "$dir/canape"

echo "Installed $("$dir/canape" version). Start with: canape"
