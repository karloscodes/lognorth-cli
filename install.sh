#!/bin/sh
# Installs north, to read your LogNorth server from your laptop.
#
#   curl -fsSL https://lognorth.com/cli | sh
#   curl -fsSL https://lognorth.com/cli | sh -s -- https://logs.yoursite.com lgn-agent-...
#
# Then it runs north connect, which asks for what the arguments leave out and
# adds LogNorth to the coding agents it finds. It installs to
# /usr/local/bin when it can write there, otherwise to ~/.local/bin. No sudo.
# Source: https://github.com/karloscodes/lognorth-cli
# Set LOGNORTH_BIN_DIR to choose the folder yourself.
set -eu

REPO="karloscodes/lognorth-cli"

say() { printf '%s\n' "$*"; }
fail() { printf 'north: %s\n' "$*" >&2; exit 1; }

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) fail "this installer knows macOS and Linux, not $(uname -s)" ;;
esac
case "$(uname -m)" in
  arm64 | aarch64) arch=arm64 ;;
  x86_64 | amd64) arch=amd64 ;;
  *) fail "no build for $(uname -m): only amd64 and arm64" ;;
esac
asset="north-$os-$arch"

if [ -n "${LOGNORTH_BIN_DIR:-}" ]; then
  dir="$LOGNORTH_BIN_DIR"
elif [ -w /usr/local/bin ]; then
  dir=/usr/local/bin
else
  dir="$HOME/.local/bin"
fi
mkdir -p "$dir"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
base="https://github.com/$REPO/releases/latest/download"
say "Downloading $asset..."
curl -fsSL -o "$tmp/$asset" "$base/$asset" || fail "could not download $base/$asset"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt" || fail "could not download the checksums"

# The binary must match the checksum the release published.
want="$(grep " $asset\$" "$tmp/checksums.txt" | cut -d' ' -f1)"
if command -v sha256sum >/dev/null 2>&1; then
  got="$(sha256sum "$tmp/$asset" | cut -d' ' -f1)"
else
  got="$(shasum -a 256 "$tmp/$asset" | cut -d' ' -f1)"
fi
[ -n "$want" ] && [ "$want" = "$got" ] || fail "the download does not match its checksum. Try again."

chmod +x "$tmp/$asset"
mv "$tmp/$asset" "$dir/north"
say "Installed $("$dir/north" version) to $dir/north"

case ":$PATH:" in
  *":$dir:"*) ;;
  *) say "Add $dir to your PATH: echo 'export PATH=\"$dir:\$PATH\"' >> ~/.$(basename "${SHELL:-sh}")rc" ;;
esac

# Connect now. curl holds stdin, so north reads the terminal directly: it asks
# for what the arguments leave out, then offers to add LogNorth to your agents.
say ""
if (exec </dev/tty) 2>/dev/null; then
  "$dir/north" connect "$@" </dev/tty || say "Connect later with: north connect"
elif [ "$#" -ge 2 ]; then
  "$dir/north" connect "$1" "$2"
else
  say "Next: north connect https://logs.yoursite.com lgn-agent-..."
  say "The agent key is in LogNorth under Settings > Developer."
fi
