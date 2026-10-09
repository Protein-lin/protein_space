#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TARGET="${1:-}"
ACTION="${2:-install}"
REMOTE_DIR="${REMOTE_DIR:-/opt/protein_space}"
DOMAIN="${DOMAIN:-linpro.top}"
[[ "$TARGET" =~ ^[a-zA-Z0-9_][a-zA-Z0-9_.@:-]*$ ]] || { echo '用法：bash scripts/deploy.sh user@host [install|update]；端口请配置 SSH 别名。' >&2; exit 2; }
[[ "$ACTION" == install || "$ACTION" == update ]] || exit 2
[[ "$REMOTE_DIR" =~ ^/[a-zA-Z0-9_./-]+$ && "$REMOTE_DIR" != / && "$REMOTE_DIR" != *..* ]] || { echo '无效 REMOTE_DIR' >&2; exit 2; }
[[ "$DOMAIN" =~ ^[a-zA-Z0-9][a-zA-Z0-9.-]*$ ]] || { echo '无效 DOMAIN' >&2; exit 2; }
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
python3 "$ROOT/scripts/package.py" "$WORK/source.tar.gz"
# Respect known_hosts and SSH config; never disable host-key verification.
remote_archive="$(ssh -o BatchMode=yes "$TARGET" 'mktemp /tmp/protein-space-source.XXXXXXXX.tar.gz')"
[[ "$remote_archive" =~ ^/tmp/protein-space-source\.[a-zA-Z0-9]+\.tar\.gz$ ]] || { echo '无效远程临时文件路径' >&2; exit 1; }
cleanup() {
  rm -rf "$WORK"
  ssh -o BatchMode=yes "$TARGET" "rm -f '$remote_archive'" >/dev/null 2>&1 || true
}
trap cleanup EXIT
scp "$WORK/source.tar.gz" "$TARGET:$remote_archive"
ssh -o BatchMode=yes "$TARGET" "bash -s -- '$REMOTE_DIR' '$DOMAIN' '$ACTION' '$remote_archive'" <<'REMOTE'
set -Eeuo pipefail
app_dir="$1"; domain="$2"; action="$3"; archive="$4"
priv=()
if [[ "$(id -u)" != 0 ]]; then sudo -n true; priv=(sudo -n); fi
"${priv[@]}" mkdir -p "$app_dir"
# Back up application source before overlaying; databases, credentials and
# private relay implementation are preserved and never uploaded from the Mac.
if [[ -f "$app_dir/scripts/install-host.sh" || -d "$app_dir/api" ]]; then
  "${priv[@]}" mkdir -p "$app_dir/backups"
  "${priv[@]}" tar --exclude='./backups' --exclude='./.git' --exclude='./bin' \
    --exclude='./.env' --exclude='./.env.*' --exclude='./mysql_data' \
    --exclude='./relay/proxy.py' --exclude='./ios-app/build' \
    -czf "$app_dir/backups/source-$(date +%Y%m%d-%H%M%S).tar.gz" -C "$app_dir" .
fi
"${priv[@]}" tar --no-same-owner -xzf "$archive" -C "$app_dir"
"${priv[@]}" env APP_DIR="$app_dir" DOMAIN="$domain" PULL=0 bash "$app_dir/scripts/$action-host.sh"
REMOTE
