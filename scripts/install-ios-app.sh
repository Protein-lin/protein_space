#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd "$(dirname "$0")/../ios-app" && pwd)"
[[ "$(uname -s)" == Darwin ]] || { echo 'iOS 客户端需要 macOS 和完整 Xcode，不能安装为 Linux 服务。' >&2; exit 1; }
xcodebuild -version >/dev/null 2>&1 || { echo '请先安装完整 Xcode，并通过 xcode-select 选择它。' >&2; exit 1; }
BUILD_DIR="${BUILD_DIR:-$ROOT/build}"
mkdir -p "$BUILD_DIR"
swiftc -module-cache-path "$BUILD_DIR/module-cache" "$ROOT/ProteinChat/ChatAPI.swift" "$ROOT/tests/main.swift" -o "$BUILD_DIR/api-tests"
"$BUILD_DIR/api-tests"
xcodebuild -project "$ROOT/ProteinChat.xcodeproj" -scheme ProteinChat \
  -sdk iphonesimulator -destination 'generic/platform=iOS Simulator' \
  -derivedDataPath "$BUILD_DIR/DerivedData" CODE_SIGNING_ALLOWED=NO build
if [[ -n "${SIMULATOR_ID:-}" ]]; then
  xcrun simctl bootstatus "$SIMULATOR_ID" -b
  xcrun simctl install "$SIMULATOR_ID" "$BUILD_DIR/DerivedData/Build/Products/Debug-iphonesimulator/ProteinChat.app"
fi
echo "构建完成：$BUILD_DIR/DerivedData/Build/Products/Debug-iphonesimulator/ProteinChat.app"
echo '真机安装请在 Xcode 中配置 Team/签名，选择 iPhone 后 Run。'
