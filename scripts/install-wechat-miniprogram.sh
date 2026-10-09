#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd "$(dirname "$0")/../wechat-miniprogram" && pwd)"
command -v node >/dev/null || { echo '请先安装 Node.js 18+' >&2; exit 1; }
node --test "$ROOT"/utils/*.test.cjs "$ROOT/tests/chat.test.cjs"
node --check "$ROOT/pages/chat/chat.js"
CLI="${WECHAT_CLI:-/Applications/wechatwebdevtools.app/Contents/MacOS/cli}"
if [[ "${CHECK_ONLY:-0}" == 1 ]]; then echo '小程序本地测试通过'; exit 0; fi
[[ -x "$CLI" ]] || { echo '请安装微信开发者工具，或通过 WECHAT_CLI 指定其 cli 路径。Linux 服务器可用 CHECK_ONLY=1 仅测试。' >&2; exit 1; }
"$CLI" open --project "$ROOT"
echo '项目已打开；请使用自己的 AppID、配置 request 合法域名并扫码预览。'
