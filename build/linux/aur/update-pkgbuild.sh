#!/usr/bin/env bash
# Stamp wingsv-dex-bin with a released version: set pkgver, replace the SKIP
# checksums with the real ones and regenerate .SRCINFO. Run it once a tag is
# published - it downloads that tag's assets to hash them - then push the stamped
# copy to the AUR repo (publishing needs an AUR account, so it stays manual).
#
# Only the -bin recipe needs this: wingsv-dex-git derives its version with a
# pkgver() function and builds from source, so it is always ready to use as is.
#
# The recipe is committed with pkgver=0.0.0 and sha256sums=SKIP because the hashes
# only exist once a release is built. AUR rejects a PKGBUILD whose .SRCINFO
# disagrees with it, so never hand-edit one without rerunning this.
#
# Usage: build/linux/aur/update-pkgbuild.sh <version> [outdir]
#   version: release version without the leading v (e.g. 0.3.0)
#   outdir:  where to write the stamped copy (default: the recipe dir in-place)
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

DEST="${OUTDIR:+$OUTDIR/wingsv-dex-bin}"
DEST="${DEST:-$HERE/wingsv-dex-bin}"
mkdir -p "$DEST"
[ "$DEST" = "$HERE/wingsv-dex-bin" ] || cp "$HERE/wingsv-dex-bin/PKGBUILD" "$DEST/PKGBUILD"

sed -i "s|^pkgver=.*|pkgver=$VERSION|" "$DEST/PKGBUILD"
sed -i "s|^sha256sums_x86_64=.*|sha256sums_x86_64=('$SHA_AMD64')|" "$DEST/PKGBUILD"
sed -i "s|^sha256sums_aarch64=.*|sha256sums_aarch64=('$SHA_ARM64')|" "$DEST/PKGBUILD"
sed -i "s|^sha256sums=.*|sha256sums=('$SHA_DESKTOP'\n            '$SHA_ICON'\n            '$SHA_LICENSE')|" "$DEST/PKGBUILD"

# .SRCINFO is what the AUR actually indexes, so it must be regenerated rather than
# written by hand. makepkg is Arch-only; skip it elsewhere and leave a note.
if command -v makepkg >/dev/null 2>&1; then
  (cd "$DEST" && makepkg --printsrcinfo > .SRCINFO)
  echo "stamped wingsv-dex-bin $VERSION (.SRCINFO regenerated)"
else
  echo "stamped wingsv-dex-bin $VERSION (no makepkg here: run 'makepkg --printsrcinfo > .SRCINFO' on Arch)"
fi
