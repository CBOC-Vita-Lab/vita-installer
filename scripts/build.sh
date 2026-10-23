#!/usr/bin/env bash
# Builds every download into dist/. Usage: scripts/build.sh 1.0.0
#
# Go 1.20 is used on purpose: it is the newest Go that still produces
# programs for Windows 7/8 and macOS 10.13/10.14.
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${1:-dev}"
NAME="CBOC-Vita-Installer"
APP="CBOC Vita Installer"
OUT="dist"
export CGO_ENABLED=0
export GOTOOLCHAIN="${GOTOOLCHAIN:-go1.20.14}"
LDFLAGS="-s -w -X main.Version=${VERSION}"

rm -rf "$OUT"
mkdir -p "$OUT/tmp"
build() { GOOS="$1" GOARCH="$2" go build -trimpath -ldflags "$LDFLAGS $4" -o "$3" .; }

echo "== macOS (universal, 10.13 and later)"
build darwin amd64 "$OUT/tmp/mac-amd64" ""
build darwin arm64 "$OUT/tmp/mac-arm64" ""
bundle="$OUT/tmp/$APP.app"
mkdir -p "$bundle/Contents/MacOS" "$bundle/Contents/Resources"
if command -v lipo >/dev/null; then
  lipo -create -output "$bundle/Contents/MacOS/$APP" "$OUT/tmp/mac-amd64" "$OUT/tmp/mac-arm64"
else
  go run ./scripts/fat "$bundle/Contents/MacOS/$APP" "$OUT/tmp/mac-amd64" "$OUT/tmp/mac-arm64"
fi
sed "s/VERSION/${VERSION}/g" packaging/Info.plist > "$bundle/Contents/Info.plist"
cp packaging/AppIcon.icns "$bundle/Contents/Resources/AppIcon.icns"
printf 'APPL????' > "$bundle/Contents/PkgInfo"
if command -v codesign >/dev/null; then
  codesign --force --deep --sign - "$bundle"
  codesign --verify --deep --strict "$bundle"
fi
if command -v ditto >/dev/null; then
  ditto -c -k --sequesterRsrc --keepParent "$bundle" "$OUT/$NAME-$VERSION-mac.zip"
else
  (cd "$OUT/tmp" && zip -qry -X "../$NAME-$VERSION-mac.zip" "$APP.app")
fi

echo "== Windows (7 and later)"
WINRES="go-winres"
command -v go-winres >/dev/null || WINRES="go run github.com/tc-hib/go-winres@v0.3.3"
WINVER="${VERSION%%-*}.0"
[[ "$WINVER" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.0$ ]] || WINVER="0.0.0.0"
$WINRES make --in packaging/winres.json --out rsrc --arch amd64,386,arm64 \
  --product-version "$WINVER" --file-version "$WINVER"
build windows amd64 "$OUT/$NAME-$VERSION-windows-x64.exe" "-H windowsgui"
build windows 386 "$OUT/$NAME-$VERSION-windows-x86.exe" "-H windowsgui"
build windows arm64 "$OUT/$NAME-$VERSION-windows-arm64.exe" "-H windowsgui"
rm -f rsrc_windows_*.syso

echo "== Linux"
for arch in amd64 arm64; do
  build linux "$arch" "$OUT/tmp/cboc-vita-installer" ""
  tar -C "$OUT/tmp" -czf "$OUT/$NAME-$VERSION-linux-$arch.tar.gz" cboc-vita-installer
  rm "$OUT/tmp/cboc-vita-installer"
done

rm -rf "$OUT/tmp"
(cd "$OUT" && { command -v sha256sum >/dev/null && sha256sum * || shasum -a 256 *; } > SHA256SUMS.txt)
ls -lh "$OUT"
