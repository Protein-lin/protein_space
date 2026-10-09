#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
[[ "$(uname -s)" == Linux && "$(id -u)" == 0 ]] || { echo '请在中转服务器使用 sudo bash scripts/install-relay.sh' >&2; exit 1; }
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
