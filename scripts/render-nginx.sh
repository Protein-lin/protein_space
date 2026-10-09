#!/usr/bin/env bash
set -Eeuo pipefail
APP_DIR="${APP_DIR:-/opt/protein_space}"
DOMAIN="${DOMAIN:-linpro.top}"
CERT_DIR="${CERT_DIR:-/etc/letsencrypt/live/$DOMAIN}"
SNIPPET="${SNIPPET:-/etc/nginx/snippets/protein-space-api.conf}"
[[ "$DOMAIN" =~ ^[a-zA-Z0-9][a-zA-Z0-9.-]*$ ]] || { echo '无效 DOMAIN' >&2; exit 1; }
for path in "$APP_DIR" "$CERT_DIR" "$SNIPPET"; do
  [[ "$path" =~ ^/[a-zA-Z0-9_./-]+$ ]] || { echo '配置路径必须是无空格的绝对路径' >&2; exit 1; }
done
names="$DOMAIN"
[[ "$DOMAIN" == www.* ]] || names="$names www.$DOMAIN"
tls=false
if [[ -f "$CERT_DIR/fullchain.pem" && -f "$CERT_DIR/privkey.pem" ]]; then tls=true; fi
if "$tls"; then
  cat <<EOF
server {
    listen 80;
    server_name $names;
    location /.well-known/acme-challenge/ { root $APP_DIR/frontend; }
    location / { return 301 https://\$host\$request_uri; }
}
EOF
fi
cat <<EOF
server {
    listen $(if "$tls"; then echo '443 ssl http2'; else echo 80; fi);
    server_name $names;
    root $APP_DIR/frontend;
    index index.html;
    client_max_body_size 24m;
EOF
if "$tls"; then
  cat <<EOF
    ssl_certificate $CERT_DIR/fullchain.pem;
    ssl_certificate_key $CERT_DIR/privkey.pem;
EOF
fi
cat <<EOF
    include $SNIPPET;
    location / { try_files \$uri \$uri/ /index.html; }
}
EOF
