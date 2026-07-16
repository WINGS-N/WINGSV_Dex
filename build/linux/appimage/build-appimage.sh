#!/usr/bin/env bash
# Build a self-contained AppImage bundling the GUI, the vkturn/xray/byedpi children and
# the whole WebKitGTK/GTK4 runtime (incl. the WebKit subprocess helpers, which linuxdeploy
# does not pick up on its own). Reproduces the recipe that works on both Arch (local) and
# Ubuntu (CI); the WebKit helper directory is found dynamically since its path differs per
# distro.
#
# Usage: build/linux/appimage/build-appimage.sh <arch> <bindir> <outdir>
#   arch:   x86_64 | aarch64
#   bindir: directory containing the built wingsv-dex, vkturn, xray and byedpi
#   outdir: where the .AppImage is written
set -euxo pipefail

# Tool arch (linuxdeploy/appimagetool require x86_64/aarch64); the output file is named
# with the friendly amd64/arm64 instead.
ARCH="${1:-x86_64}"
BINDIR="$(readlink -f "${2:-bin}")"
OUTDIR="$(readlink -f "${3:-dist}")"
case "$ARCH" in
  x86_64) PKGARCH=amd64 ;;
  aarch64) PKGARCH=arm64 ;;
  *) PKGARCH="$ARCH" ;;
esac
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"

export APPIMAGE_EXTRACT_AND_RUN=1   # AppImage tools cannot FUSE-mount in CI/sandboxes
export NO_STRIP=1                   # bundled strip chokes on modern (.relr.dyn) ELF
export DEPLOY_GTK_VERSION=4         # WebKitGTK 6.0 is GTK4
export ARCH

WORK="$HERE/build"
AD="$WORK/wingsv-dex-${ARCH}.AppDir"
rm -rf "$AD"
mkdir -p "$AD/usr/bin" "$AD/apprun-hooks"

cp "$BINDIR/wingsv-dex" "$AD/usr/bin/wingsv-dex"
cp "$BINDIR/vkturn" "$AD/usr/bin/vkturn"
cp "$BINDIR/xray" "$AD/usr/bin/xray"
cp "$BINDIR/byedpi" "$AD/usr/bin/byedpi"
cp "$ROOT/build/appicon.png" "$AD/wingsv-dex.png"
cp "$ROOT/build/linux/wingsv-dex.desktop" "$AD/wingsv-dex.desktop"

# linuxdeploy + its GTK plugin (bundle libwebkitgtk, gdk-pixbuf loaders, GIO modules...).
cd "$WORK"
fetch() { curl -fsSL -o "$1" "$2"; chmod +x "$1"; }
fetch "linuxdeploy-${ARCH}.AppImage" "https://github.com/linuxdeploy/linuxdeploy/releases/download/continuous/linuxdeploy-${ARCH}.AppImage"
fetch "linuxdeploy-plugin-gtk.sh" "https://raw.githubusercontent.com/linuxdeploy/linuxdeploy-plugin-gtk/master/linuxdeploy-plugin-gtk.sh"
fetch "appimagetool-${ARCH}.AppImage" "https://github.com/AppImage/appimagetool/releases/download/continuous/appimagetool-${ARCH}.AppImage"
export PATH="$WORK:$PATH"

"$WORK/linuxdeploy-${ARCH}.AppImage" --appdir "$AD" --plugin gtk \
  --desktop-file "$AD/wingsv-dex.desktop" --icon-file "$AD/wingsv-dex.png"

# WebKitGTK spawns WebKitNetworkProcess / WebKitWebProcess from a libexec dir linuxdeploy
# does not follow. Bundle that directory and point WebKitGTK at it inside the AppImage.
WK_HELPER="$(dirname "$(find /usr/lib /usr/lib64 /usr/libexec -name WebKitNetworkProcess 2>/dev/null | grep -E 'webkitgtk-6.0' | head -1)")"
if [ -n "$WK_HELPER" ] && [ -d "$WK_HELPER" ]; then
  mkdir -p "$AD/usr/lib/webkitgtk-6.0"
  cp -rn "$WK_HELPER/." "$AD/usr/lib/webkitgtk-6.0/"
fi
cat > "$AD/apprun-hooks/webkit-exec-path.sh" <<'HOOK'
export WEBKIT_EXEC_PATH="${APPDIR}/usr/lib/webkitgtk-6.0"
export WEBKIT_INJECTED_BUNDLE_PATH="${APPDIR}/usr/lib/webkitgtk-6.0/injected-bundle"
HOOK

mkdir -p "$OUTDIR"
"$WORK/appimagetool-${ARCH}.AppImage" "$AD" "$OUTDIR/wingsv-dex-${PKGARCH}.AppImage"
echo "AppImage: $OUTDIR/wingsv-dex-${PKGARCH}.AppImage"
