#!/usr/bin/env bash
set -Eeuo pipefail
case "${1:-}" in
relay)
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
[[ "$(uname -s)" == Linux && "$(id -u)" == 0 ]] || { echo '请在中转服务器使用 sudo bash scripts/deploy.sh install relay' >&2; exit 1; }
[[ "$ROOT" =~ ^/[a-zA-Z0-9_./-]+$ ]] || { echo '项目路径不能含空格' >&2; exit 1; }
[[ -f "$ROOT/relay/proxy.py" ]] || { echo '缺少本机私有 relay/proxy.py；此文件不进入公开部署包，请单独配置于内网中转机。' >&2; exit 1; }
command -v python3 >/dev/null || { apt-get update; apt-get install -y python3; }
python3 -m py_compile "$ROOT/relay/proxy.py"
cat >/etc/systemd/system/protein-relay.service <<EOF
[Unit]
Description=Protein Space internal relay
After=network-online.target
Wants=network-online.target
[Service]
WorkingDirectory=$ROOT
ExecStart=/usr/bin/python3 $ROOT/relay/proxy.py
Restart=always
RestartSec=3
[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable protein-relay
systemctl restart protein-relay
systemctl is-active --quiet protein-relay
echo 'Relay 服务已启动；请在中转机检查已配置端口和上游。'
;;
ios-app)
ROOT="$(cd "$(dirname "$0")/../../ios-app" && pwd)"
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
;;
wechat-miniprogram)
ROOT="$(cd "$(dirname "$0")/../../wechat-miniprogram" && pwd)"
command -v node >/dev/null || { echo '请先安装 Node.js 18+' >&2; exit 1; }
node --test "$ROOT"/utils/*.test.cjs "$ROOT/tests/chat.test.cjs"
node --check "$ROOT/pages/chat/chat.js"
CLI="${WECHAT_CLI:-/Applications/wechatwebdevtools.app/Contents/MacOS/cli}"
if [[ "${CHECK_ONLY:-0}" == 1 ]]; then echo '小程序本地测试通过'; exit 0; fi
[[ -x "$CLI" ]] || { echo '请安装微信开发者工具，或通过 WECHAT_CLI 指定其 cli 路径。Linux 服务器可用 CHECK_ONLY=1 仅测试。' >&2; exit 1; }
"$CLI" open --project "$ROOT"
echo '项目已打开；请使用自己的 AppID、配置 request 合法域名并扫码预览。'
;;
*) echo "未知客户端组件" >&2; exit 2;;
esac
