#!/usr/bin/env bash
# Build an unsigned development DeepShift.app and a zip that keeps the
# executable bit.
#
#   scripts/build-macos-app.sh [arm64|amd64]     (default: arm64)
#
# Output: dist/DeepShift.app and dist/DeepShift-macos-<arch>.zip
#
# On macOS: builds natively, applies an ad-hoc code signature to the whole
# bundle (codesign -s -; this is NOT a Developer ID signature and the app is
# not notarised) and zips it with ditto.
# Elsewhere: cross-compiles with CGO_ENABLED=0, skips signing and zips with
# zip -y. That path only checks the bundle layout; ship the macOS-built one.
set -euo pipefail

arch="${1:-arm64}"
case "$arch" in
  arm64 | amd64) ;;
  *) echo "usage: $0 [arm64|amd64]" >&2; exit 2 ;;
esac

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dist="$root/dist"
app="$dist/DeepShift.app"
zip="$dist/DeepShift-macos-$arch.zip"

revision="$(git -C "$root" describe --tags --always --dirty 2>/dev/null || echo unknown)"
build_number="${GITHUB_RUN_NUMBER:-1}"
# Oldest macOS supported: Go 1.27's own minimum. The plist and the binary's
# LC_BUILD_VERSION must agree, so both come from here.
min_macos=13.0

rm -rf "$app" "$zip"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"

if [[ "$(uname -s)" == Darwin ]]; then
  # With cgo, the system linker would otherwise stamp the build machine's
  # macOS version as the minimum.
  export MACOSX_DEPLOYMENT_TARGET="$min_macos"
else
  export CGO_ENABLED=0 # Go's internal linker stamps 13.0 itself
fi
(cd "$root" && GOOS=darwin GOARCH="$arch" go build -trimpath -o "$app/Contents/MacOS/deepshift" ./cmd/deepshift)

sed -e "s|@BUILD@|$build_number|g" -e "s|@REVISION@|$revision|g" -e "s|@MIN_MACOS@|$min_macos|g" \
  "$root/packaging/macos/Info.plist" > "$app/Contents/Info.plist"
printf 'APPL????' > "$app/Contents/PkgInfo"

if command -v codesign > /dev/null; then
  codesign --force --sign - "$app"
  codesign --verify --strict --verbose=2 "$app"
else
  echo "codesign not found: bundle left unsigned (layout check only)" >&2
fi

if command -v ditto > /dev/null; then
  ditto -c -k --keepParent "$app" "$zip"
else
  (cd "$dist" && zip -qry "$zip" DeepShift.app)
fi

echo "built  $app ($arch, $revision)"
echo "packed $zip"
