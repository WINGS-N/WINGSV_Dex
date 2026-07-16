#!/usr/bin/env bash
# Move the two release recipes onto a new version: set pkgver on wingsv-dex and
# wingsv-dex-bin, replace the -bin SKIP checksums with the real ones, and
# regenerate .SRCINFO. Run it once a tag is published - it downloads that tag's
# assets to hash them - then push the stamped copies to the AUR repos (publishing
# needs an AUR account, so it stays manual).
#
# wingsv-dex-git is left alone: it tracks main and derives its version with a
# pkgver() function, so there is nothing to stamp.
#
# Only -bin carries checksums (its sources are release assets); the source recipes
# build from a git tag, which makepkg checks out by name. AUR rejects a PKGBUILD
# whose .SRCINFO disagrees with it, so never hand-edit one without rerunning this.
#
# Usage: build/linux/aur/update-pkgbuild.sh <version> [outdir]
#   version: release version without the leading v (e.g. 0.3.0)
#   outdir:  where to write the stamped copies (default: the recipe dirs in-place)
set -euo pipefail

VERSION="${1:?usage: update-pkgbuild.sh <version> [outdir]}"
VERSION="${VERSION#v}"
HERE="$(cd "$(dirname "$0")" && pwd)"
OUTDIR="${2:-}"
BASE_URL="https://github.com/WINGS-N/WINGSV_DeX"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

fetch_sha() { # url -> sha256, on stdout
  curl -fsSL "$1" -o "$WORK/asset"
  sha256sum "$WORK/asset" | cut -d' ' -f1
}

echo "hashing v$VERSION assets..."
SHA_AMD64="$(fetch_sha "$BASE_URL/releases/download/v$VERSION/wingsv-dex-linux-amd64.tar.gz")"
SHA_ARM64="$(fetch_sha "$BASE_URL/releases/download/v$VERSION/wingsv-dex-linux-arm64.tar.gz")"
SHA_DESKTOP="$(fetch_sha "$BASE_URL/raw/v$VERSION/build/linux/wingsv-dex.desktop")"
SHA_ICON="$(fetch_sha "$BASE_URL/raw/v$VERSION/build/appicon.png")"
SHA_LICENSE="$(fetch_sha "$BASE_URL/raw/v$VERSION/LICENSE")"

stamp() { # recipe name
  local name="$1" dest
  dest="${OUTDIR:+$OUTDIR/$name}"
  dest="${dest:-$HERE/$name}"
  mkdir -p "$dest"
  [ "$dest" = "$HERE/$name" ] || cp "$HERE/$name/PKGBUILD" "$dest/PKGBUILD"

  sed -i "s|^pkgver=.*|pkgver=$VERSION|" "$dest/PKGBUILD"
  if [ "$name" = "wingsv-dex-bin" ]; then
    sed -i "s|^sha256sums_x86_64=.*|sha256sums_x86_64=('$SHA_AMD64')|" "$dest/PKGBUILD"
    sed -i "s|^sha256sums_aarch64=.*|sha256sums_aarch64=('$SHA_ARM64')|" "$dest/PKGBUILD"
    sed -i "s|^sha256sums=.*|sha256sums=('$SHA_DESKTOP'\n            '$SHA_ICON'\n            '$SHA_LICENSE')|" "$dest/PKGBUILD"
  fi

  # .SRCINFO is what the AUR actually indexes, so it must be regenerated rather
  # than written by hand. makepkg is Arch-only; skip it elsewhere and leave a note.
  if command -v makepkg >/dev/null 2>&1; then
    (cd "$dest" && makepkg --printsrcinfo > .SRCINFO)
    echo "stamped $name $VERSION (.SRCINFO regenerated)"
  else
    echo "stamped $name $VERSION (no makepkg here: run 'makepkg --printsrcinfo > .SRCINFO' on Arch)"
  fi
}

stamp wingsv-dex
stamp wingsv-dex-bin
