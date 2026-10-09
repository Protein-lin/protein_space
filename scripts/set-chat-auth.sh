#!/usr/bin/env bash
set -Eeuo pipefail
MODE="${1:-}"
case "$MODE" in on) VALUE=true;; off) VALUE=false;; *) echo '用法：sudo bash scripts/set-chat-auth.sh off|on' >&2; exit 2;; esac
[[ "$(id -u)" == 0 ]] || { echo '请使用 sudo 执行' >&2; exit 1; }
ENV_FILE="${ENV_DIR:-/etc/protein-space}/api.env"
[[ -f "$ENV_FILE" ]] || { echo "请先安装 API：缺少 $ENV_FILE" >&2; exit 1; }
TEMP="$(mktemp "${ENV_FILE}.XXXXXX")"
trap 'rm -f "$TEMP"' EXIT
awk '!/^[[:space:]]*AUTH_REQUIRED=/' "$ENV_FILE" > "$TEMP"
printf 'AUTH_REQUIRED=%s\n' "$VALUE" >> "$TEMP"
chmod 600 "$TEMP"
mv "$TEMP" "$ENV_FILE"
systemctl restart protein-api
systemctl is-active --quiet protein-api
echo "AUTH_REQUIRED=$VALUE 已写入，API 已重启。请确认已更新为支持免登录聊天的版本。"
