#!/usr/bin/env bash
set -Eeuo pipefail

APP_DIR="${APP_DIR:-/opt/protein_space}"
BIN_DIR="$APP_DIR/bin"
BRANCH="${BRANCH:-main}"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "请使用 root 或 sudo 执行：sudo bash scripts/update-host.sh" >&2
  exit 1
fi

if [[ ! -d "$APP_DIR/.git" ]]; then
  echo "项目 Git 目录不存在：$APP_DIR" >&2
  exit 1
fi

mkdir -p "$BIN_DIR"

echo "拉取最新代码..."
git -C "$APP_DIR" pull --ff-only origin "$BRANCH"

echo "编译 API..."
(
  cd "$APP_DIR/api"
  go mod download
  go test ./...
  CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$BIN_DIR/api" .
)

echo "编译 Agent..."
(
  cd "$APP_DIR/agent"
  go mod download
  go test ./...
  CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$BIN_DIR/agent" .
)

chmod 755 "$BIN_DIR/api" "$BIN_DIR/agent"

echo "重启服务..."
systemctl restart protein-agent protein-api
if systemctl is-active --quiet nginx; then
  systemctl reload nginx
fi

echo "更新完成。"
