#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export APP_DIR="${APP_DIR:-$ROOT}"
ENV_DIR="${ENV_DIR:-/etc/protein-space}"
SYSTEMD_DIR="${SYSTEMD_DIR:-/etc/systemd/system}"
NGINX_DIR="${NGINX_DIR:-/etc/nginx}"
LOCK_FILE="${LOCK_FILE:-/var/lock/protein-space-deploy.lock}"
export DOMAIN="${DOMAIN:-linpro.top}"
BIN_DIR="$APP_DIR/bin"
component="${1:-all}"
action="${2:-install}"
case "$component" in all|api|agent|frontend|nginx|mysql) ;; *) echo '未知组件' >&2; exit 2;; esac
case "$action" in install|update) ;; *) echo '操作应为 install 或 update' >&2; exit 2;; esac
[[ "$(uname -s)" == Linux && "$(id -u)" == 0 ]] || { echo '需要 Ubuntu/Debian + systemd，请使用 sudo bash 执行。' >&2; exit 1; }
for path in "$APP_DIR" "$ENV_DIR" "$SYSTEMD_DIR" "$NGINX_DIR" "$LOCK_FILE"; do
  [[ "$path" =~ ^/[a-zA-Z0-9_./-]+$ ]] || { echo '目录必须是无空格的绝对路径' >&2; exit 1; }
done
command -v flock >/dev/null || { echo '请先安装 util-linux' >&2; exit 1; }
exec 9>"$LOCK_FILE"
flock -n 9 || { echo '另一次部署正在执行' >&2; exit 1; }
if [[ "$action" == update && "${PULL:-0}" == 1 ]]; then
  [[ -d "$APP_DIR/.git" ]] || { echo 'PULL=1 需要 Git 检出目录' >&2; exit 1; }
  [[ -z "$(git -C "$APP_DIR" status --porcelain)" ]] || { echo '服务器存在本地改动，停止拉取' >&2; exit 1; }
  git -C "$APP_DIR" pull --ff-only origin "${BRANCH:-main}"
fi

install_packages() {
  export DEBIAN_FRONTEND=noninteractive
  apt-get update
  apt-get install -y "$@"
}

prepare_env() {
  mkdir -p "$ENV_DIR" "$BIN_DIR"
  local missing=0
  umask 077
  if [[ "$component" == all || "$component" == mysql ]]; then
    if [[ ! -f "$APP_DIR/.env" ]]; then
      local root_password db_password
      root_password="$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')"
      db_password="$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')"
      printf 'MYSQL_ROOT_PASSWORD=%s\nMYSQL_PASSWORD=%s\n' "$root_password" "$db_password" > "$APP_DIR/.env"
    fi
  fi
  if [[ "$component" == all || "$component" == agent ]]; then
    if [[ ! -f "$ENV_DIR/agent.env" ]]; then
      printf '%s\n' 'AGENT_ADDR=127.0.0.1:8090' 'AGENT_MODEL=gpt-5.6-terra' 'AGENT_UPSTREAM_URL=https://REPLACE_ME/v1/chat/completions' 'AGENT_API_KEY=REPLACE_ME' > "$ENV_DIR/agent.env"
    fi
    if grep -qE 'REPLACE_ME|替换为' "$ENV_DIR/agent.env"; then
      echo "请在 $ENV_DIR/agent.env 填写模型地址及 Key。" >&2; missing=1
    fi
  fi
  if [[ "$component" == all || "$component" == api ]]; then
    if [[ ! -f "$ENV_DIR/api.env" ]]; then
      local db_password encryption
      db_password="$(sed -n 's/^MYSQL_PASSWORD=//p' "$APP_DIR/.env" 2>/dev/null || true)"
      # Only generated hex passwords can be copied without DSN/Compose quoting.
      [[ "$db_password" =~ ^[a-f0-9]{64}$ ]] || db_password=REPLACE_ME
      encryption="$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')"
      printf '%s\n' 'API_ADDR=127.0.0.1:8080' 'AGENT_URL=http://127.0.0.1:8090' \
        "MYSQL_DSN=waf:$db_password@tcp(127.0.0.1:3306)/waf_agent?parseTime=true&charset=utf8mb4" \
        "APP_ENCRYPTION_KEY=$encryption" 'AUTH_WHITELIST_IPS=127.0.0.1/32,::1/128' > "$ENV_DIR/api.env"
    fi
    if grep -qE 'REPLACE_ME|替换为' "$ENV_DIR/api.env"; then
      echo "请在 $ENV_DIR/api.env 填写数据库配置。" >&2; missing=1
    fi
  fi
  umask 022
  [[ "$missing" == 0 ]] || { echo '配置已准备；填写后再次执行相同安装命令。' >&2; exit 2; }
}

health() {
  local url="$1" attempt
  for attempt in {1..30}; do
    if curl --fail --silent --max-time 2 "$url" >/dev/null; then return; fi
    sleep 1
  done
  echo "健康检查失败：$url" >&2; return 1
}

mysql_start() {
  command -v docker >/dev/null || install_packages docker.io
  if ! docker compose version >/dev/null 2>&1; then install_packages docker-compose-v2; fi
  systemctl enable --now docker
  [[ -f "$APP_DIR/.env" ]] || { echo '缺少 MySQL .env' >&2; return 1; }
  if grep -qE 'REPLACE_ME|替换为|change-me' "$APP_DIR/.env"; then echo '请填写真实 MySQL 密码' >&2; return 1; fi
  (cd "$APP_DIR" && docker compose up -d mysql)
  # No volume deletion, forced image pull or schema reinitialization on update.
  local attempt
  for attempt in {1..45}; do
    if (cd "$APP_DIR" && docker compose exec -T mysql sh -c 'MYSQL_PWD="$MYSQL_PASSWORD" mysql -uwaf -D waf_agent -Nse "SELECT 1"' >/dev/null 2>&1); then return; fi
    sleep 2
  done
  echo 'MySQL 未在 90 秒内就绪' >&2; return 1
}

build_go() {
  local name="$1"
  mkdir -p "$BIN_DIR"
  (cd "$APP_DIR/$name" && go test ./... && go vet ./... && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o "$BIN_DIR/$name.next" .)
  chmod 755 "$BIN_DIR/$name.next"
}

activate_go() {
  local name="$1" port="$2" endpoint="$3"
  [[ -f "$ENV_DIR/$name.env" ]] || { echo "缺少 $ENV_DIR/$name.env" >&2; return 1; }
  if [[ "$action" == install ]]; then
    mkdir -p "$SYSTEMD_DIR"
    cat >"$SYSTEMD_DIR/protein-$name.service" <<EOF
[Unit]
Description=Protein Space $name
After=network-online.target docker.service
Wants=network-online.target
[Service]
WorkingDirectory=$APP_DIR
EnvironmentFile=$ENV_DIR/$name.env
ExecStart=$BIN_DIR/$name
Restart=always
RestartSec=3
[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
    systemctl enable "protein-$name"
  fi
  if [[ -f "$BIN_DIR/$name" ]]; then cp -p "$BIN_DIR/$name" "$BIN_DIR/$name.previous"; fi
  mv "$BIN_DIR/$name.next" "$BIN_DIR/$name"
  if systemctl restart "protein-$name" && health "http://127.0.0.1:$port/$endpoint"; then
    echo "$name 已就绪"
  else
    if [[ -f "$BIN_DIR/$name.previous" ]]; then
      cp -p "$BIN_DIR/$name.previous" "$BIN_DIR/$name.rollback"
      mv "$BIN_DIR/$name.rollback" "$BIN_DIR/$name"
      systemctl restart "protein-$name" || true
      echo "$name 已恢复上一版二进制" >&2
    fi
    return 1
  fi
}

nginx_apply() {
  local site="$NGINX_DIR/sites-available/protein-space" snippet="$NGINX_DIR/snippets/protein-space-api.conf" backup
  backup="$(mktemp -d)"
  mkdir -p "$NGINX_DIR/sites-available" "$NGINX_DIR/sites-enabled" "$NGINX_DIR/snippets"
  [[ ! -e "$site" ]] || cp -p "$site" "$backup/site"
  [[ ! -e "$snippet" ]] || cp -p "$snippet" "$backup/snippet"
  if ! SNIPPET="$snippet" bash "$APP_DIR/scripts/render-nginx.sh" > "$backup/rendered"; then rm -rf "$backup"; return 1; fi
  cp "$backup/rendered" "$site"
  cp "$APP_DIR/nginx/api-locations.conf" "$snippet"
  local new_link=0
  if [[ ! -e "$NGINX_DIR/sites-enabled/protein-space" && ! -L "$NGINX_DIR/sites-enabled/protein-space" ]]; then
    ln -s "$site" "$NGINX_DIR/sites-enabled/protein-space"; new_link=1
  fi
  if ! nginx -t || ! systemctl enable --now nginx || ! systemctl reload nginx; then
    if [[ -f "$backup/site" ]]; then cp -p "$backup/site" "$site"; else rm -f "$site"; fi
    if [[ -f "$backup/snippet" ]]; then cp -p "$backup/snippet" "$snippet"; else rm -f "$snippet"; fi
    [[ "$new_link" == 0 ]] || rm -f "$NGINX_DIR/sites-enabled/protein-space"
    rm -rf "$backup"; return 1
  fi
  rm -rf "$backup"
}

if [[ "$action" == install ]]; then prepare_env; fi
if [[ "$component" == all || "$component" == api || "$component" == agent ]]; then
  if [[ "$action" == install ]]; then install_packages golang-go ca-certificates curl; fi
  command -v go >/dev/null || { echo '请先执行 scripts/install-host.sh 安装 Go' >&2; exit 1; }
fi
if [[ "$component" == all || "$component" == nginx || "$component" == frontend ]]; then
  if [[ "$action" == install ]]; then install_packages nginx ca-certificates curl; fi
fi
# Build both services before activating either of them.
if [[ "$component" == all || "$component" == agent ]]; then build_go agent; fi
if [[ "$component" == all || "$component" == api ]]; then build_go api; fi
if [[ "$component" == all || "$component" == mysql ]]; then mysql_start; fi
if [[ "$component" == all || "$component" == agent ]]; then activate_go agent 8090 v1/health; fi
if [[ "$component" == all || "$component" == api ]]; then activate_go api 8080 api/health; fi
if [[ "$component" == all || "$component" == frontend || "$component" == nginx ]]; then nginx_apply; fi
echo "$component $action 完成。"
