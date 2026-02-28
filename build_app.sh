#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
APP_NAME="FrameScope"
BUNDLE_DIR="$ROOT_DIR/dist/$APP_NAME.app"
CONTENTS_DIR="$BUNDLE_DIR/Contents"
MACOS_DIR="$CONTENTS_DIR/MacOS"
RESOURCES_DIR="$CONTENTS_DIR/Resources"
ICON_SOURCE="$ROOT_DIR/withoutbg-FrameScopeLogo.png"
ICONSET_DIR="$ROOT_DIR/FrameScope.iconset"
ICNS_PATH="$ROOT_DIR/FrameScope.icns"
PLIST_PATH="$CONTENTS_DIR/Info.plist"
BINARY_PATH="$MACOS_DIR/$APP_NAME"

mkdir -p "$ROOT_DIR/dist"
rm -rf "$BUNDLE_DIR"
mkdir -p "$MACOS_DIR" "$RESOURCES_DIR"

if [[ ! -f "$ICON_SOURCE" ]]; then
  echo "Missing icon source: $ICON_SOURCE" >&2
  exit 1
fi

echo "Building $APP_NAME binary..."
env GOCACHE="$ROOT_DIR/.gocache" go build -o "$BINARY_PATH" .
chmod +x "$BINARY_PATH"

echo "Preparing icon assets..."
rm -rf "$ICONSET_DIR"
mkdir -p "$ICONSET_DIR"

sips -z 16 16   "$ICON_SOURCE" --out "$ICONSET_DIR/icon_16x16.png" >/dev/null
sips -z 32 32   "$ICON_SOURCE" --out "$ICONSET_DIR/icon_16x16@2x.png" >/dev/null
sips -z 32 32   "$ICON_SOURCE" --out "$ICONSET_DIR/icon_32x32.png" >/dev/null
sips -z 64 64   "$ICON_SOURCE" --out "$ICONSET_DIR/icon_32x32@2x.png" >/dev/null
sips -z 128 128 "$ICON_SOURCE" --out "$ICONSET_DIR/icon_128x128.png" >/dev/null
sips -z 256 256 "$ICON_SOURCE" --out "$ICONSET_DIR/icon_128x128@2x.png" >/dev/null
sips -z 256 256 "$ICON_SOURCE" --out "$ICONSET_DIR/icon_256x256.png" >/dev/null
sips -z 512 512 "$ICON_SOURCE" --out "$ICONSET_DIR/icon_256x256@2x.png" >/dev/null
sips -z 512 512 "$ICON_SOURCE" --out "$ICONSET_DIR/icon_512x512.png" >/dev/null
cp "$ICON_SOURCE" "$ICONSET_DIR/icon_512x512@2x.png"

ICON_FILE_KEY=""
if iconutil -c icns "$ICONSET_DIR" -o "$ICNS_PATH" >/dev/null 2>&1; then
  cp "$ICNS_PATH" "$RESOURCES_DIR/$APP_NAME.icns"
  ICON_FILE_KEY="    <key>CFBundleIconFile</key>
    <string>$APP_NAME</string>"
  echo "Created bundle icon: $RESOURCES_DIR/$APP_NAME.icns"
else
  cp "$ICON_SOURCE" "$RESOURCES_DIR/$APP_NAME.png"
  echo "iconutil failed; bundling PNG fallback only." >&2
fi

cat > "$PLIST_PATH" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleDevelopmentRegion</key>
    <string>en</string>
    <key>CFBundleDisplayName</key>
    <string>$APP_NAME</string>
    <key>CFBundleExecutable</key>
    <string>$APP_NAME</string>
    <key>CFBundleIdentifier</key>
    <string>com.danielthiem.framescope</string>
${ICON_FILE_KEY}
    <key>CFBundleInfoDictionaryVersion</key>
    <string>6.0</string>
    <key>CFBundleName</key>
    <string>$APP_NAME</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>1.0</string>
    <key>CFBundleVersion</key>
    <string>1</string>
    <key>LSMinimumSystemVersion</key>
    <string>12.0</string>
    <key>NSHighResolutionCapable</key>
    <true/>
</dict>
</plist>
EOF

echo "Built app bundle at: $BUNDLE_DIR"
