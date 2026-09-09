#!/usr/bin/env bash
set -Eeuo pipefail

APP_DIR="${APP_DIR:-/opt/protein_space}"
BIN_DIR="$APP_DIR/bin"
ENV_DIR="${ENV_DIR:-/etc/protein-space}"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "请使用 root 或 sudo 执行：sudo bash scripts/install-host.sh" >&2
  exit 1
fi
if [[ ! -d "$APP_DIR" ]]; then
  echo "项目目录不存在：$APP_DIR" >&2
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y nginx golang-go ca-certificates curl docker.io docker-compose-plugin
systemctl enable --now docker nginx

mkdir -p "$BIN_DIR" "$ENV_DIR"
NEED_CONFIG=0
if [[ ! -f "$ENV_DIR/agent.env" ]]; then
  cat >"$ENV_DIR/agent.env" <<'EOF'
AGENT_ADDR=:8090
AGENT_MODEL=gpt-5.5
AGENT_UPSTREAM_URL=https://替换为模型服务地址/v1/chat/completions
AGENT_API_KEY=替换为新的模型服务Key
EOF
  echo "已创建 $ENV_DIR/agent.env，请先填写模型服务配置。"
  NEED_CONFIG=1
fi
if [[ ! -f "$ENV_DIR/api.env" ]]; then
  cat >"$ENV_DIR/api.env" <<'EOF'
API_ADDR=:8080
AGENT_URL=http://127.0.0.1:8090
MYSQL_DSN=waf:替换为MySQL业务密码@tcp(127.0.0.1:3306)/waf_agent?parseTime=true&charset=utf8mb4
APP_ENCRYPTION_KEY=替换为32字节以上随机密钥
AUTH_REQUIRED=true
SECURE_COOKIE=false
AUTH_WHITELIST_IPS=127.0.0.1/32,::1/128
EOF
  echo "已创建 $ENV_DIR/api.env，请先填写数据库和加密配置。"
  NEED_CONFIG=1
fi
chmod 600 "$ENV_DIR"/*.env
if [[ "$NEED_CONFIG" -eq 1 ]]; then
  echo "请填写环境文件后重新执行本脚本。"
  exit 0
fi

cd "$APP_DIR"
if [[ -f .env ]]; then docker compose up -d mysql; else echo "未找到 $APP_DIR/.env，跳过 MySQL 启动。"; fi

echo "编译 API..."
(cd api && go mod download && go test ./... && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$BIN_DIR/api" .)
echo "编译 Agent..."
(cd agent && go test ./... && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$BIN_DIR/agent" .)
chmod 755 "$BIN_DIR/api" "$BIN_DIR/agent"

cat >/etc/systemd/system/protein-agent.service <<EOF
[Unit]
Description=Protein Space Agent
After=network-online.target
Wants=network-online.target
[Service]
WorkingDirectory=$APP_DIR
EnvironmentFile=$ENV_DIR/agent.env
ExecStart=$BIN_DIR/agent
Restart=always
RestartSec=3
[Install]
WantedBy=multi-user.target
EOF

cat >/etc/systemd/system/protein-api.service <<EOF
[Unit]
Description=Protein Space API
After=network-online.target docker.service
Wants=network-online.target
[Service]
WorkingDirectory=$APP_DIR
EnvironmentFile=$ENV_DIR/api.env
ExecStart=$BIN_DIR/api
Restart=always
RestartSec=3
[Install]
WantedBy=multi-user.target
EOF

cat >/etc/nginx/sites-available/protein-space <<EOF
server {
    listen 80;
    server_name _;
    root $APP_DIR/frontend;
    index index.html;
    location / { try_files \$uri \$uri/ /index.html; }
    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 1h;
        proxy_send_timeout 1h;
        proxy_set_header Connection "";
    }
}
EOF
ln -sfn /etc/nginx/sites-available/protein-space /etc/nginx/sites-enabled/protein-space
rm -f /etc/nginx/sites-enabled/default
nginx -t
systemctl daemon-reload
systemctl enable --now protein-agent protein-api
systemctl reload nginx

echo "安装完成。检查：curl http://127.0.0.1:8080/api/health"
