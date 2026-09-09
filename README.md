# Protein Space / WAF Agent

支持 SSE 流式对话、模型选择、用户登录、`X-API-Key`、MySQL 持久化和 IP 白名单。

生产部署采用以下结构，不再拉取 Nginx 或 Go Docker 镜像：

```text
宿主机 Nginx → 宿主机 Go API (:8080) → 宿主机 Go Agent (:8090)
                                      ↓
                             Docker MySQL (127.0.0.1:3306)
```

## 目录

```text
frontend/       页面和 SSE 客户端
api/            Go API、登录、API Key、白名单和 MySQL 持久化
agent/          Go Agent 和上游模型 SSE
mysql/init/     MySQL 初始化表结构
nginx/           Nginx 配置参考
docker-compose.yml 仅运行 MySQL
```

## Ubuntu 24.04 / 阿里云 ECS 部署

以下命令在服务器 `/opt/protein_space` 执行。安全组只开放 `80/tcp`；不要把 `3306`、`8080`、`8090` 暴露到公网。

### 1. 一键安装（推荐）

脚本会安装 Nginx、Go、Docker，启动 MySQL，编译 API/Agent，创建 systemd 服务并配置宿主机 Nginx。它不会覆盖已有的 `/etc/protein-space/*.env`。

```bash
cd /opt/protein_space
sudo bash scripts/install-host.sh
```

首次执行如果没有环境文件，脚本会创建模板。填写真实值后重新执行脚本即可：

```bash
sudo vi /etc/protein-space/agent.env
sudo vi /etc/protein-space/api.env
sudo bash scripts/install-host.sh
```

检查：

```bash
systemctl status protein-agent protein-api nginx --no-pager
curl http://127.0.0.1:8080/api/health
curl http://127.0.0.1:8090/v1/health
```

脚本支持自定义项目目录：

```bash
sudo APP_DIR=/opt/protein_space bash scripts/install-host.sh
```

### 2. 手动安装宿主机依赖

```bash
sudo apt update
sudo apt install -y git nginx golang-go ca-certificates curl
# 如果系统还没有 Docker，再安装 docker.io；已有 Docker CE/containerd.io 时不要混装 docker.io
if ! command -v docker >/dev/null 2>&1; then sudo apt install -y docker.io; fi
if ! docker compose version >/dev/null 2>&1; then sudo apt install -y docker-compose-plugin || sudo apt install -y docker-compose-v2; fi
sudo systemctl enable --now docker nginx
sudo usermod -aG docker "$USER"
# 重新登录一次使 docker 用户组生效
go version
nginx -v
docker compose version
```

### 3. 拉取代码并启动 MySQL

```bash
sudo mkdir -p /opt
sudo git clone git@github.com:Protein-lin/protein_space.git /opt/protein_space
sudo chown -R "$USER":"$USER" /opt/protein_space
cd /opt/protein_space

cat > .env <<'EOF'
MYSQL_ROOT_PASSWORD=请替换为强随机Root密码
MYSQL_PASSWORD=请替换为强随机业务密码
EOF
chmod 600 .env
docker compose up -d mysql
docker compose ps
```

`docker-compose.yml` 只会拉取 `mysql:8.0`。首次启动自动执行 `mysql/init/001_schema.sql`；已有数据卷不会重复执行初始化脚本。更新表结构时执行：

```bash
set -a; . ./.env; set +a
docker compose exec -T mysql mysql -uroot -p"$MYSQL_ROOT_PASSWORD" waf_agent < mysql/init/001_schema.sql
```

### 4. 编译 API 和 Agent

```bash
cd /opt/protein_space
mkdir -p bin

(cd api && go mod download && go test ./... && \
  CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ../bin/api .)
(cd agent && go test ./... && \
  CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ../bin/agent .)

file bin/api bin/agent
```

服务器是 ARM 时可直接本地编译；交叉编译 ARM 使用 `GOOS=linux GOARCH=arm64`，x86_64 使用 `GOOS=linux GOARCH=amd64`。查看架构：`uname -m`。

### 5. 配置服务密钥

```bash
sudo install -d -m 750 /etc/protein-space
sudo tee /etc/protein-space/agent.env >/dev/null <<'EOF'
AGENT_ADDR=:8090
AGENT_MODEL=gpt-5.5
AGENT_UPSTREAM_URL=https://你的模型服务地址/v1/chat/completions
AGENT_API_KEY=请替换为新的模型服务Key
EOF

sudo tee /etc/protein-space/api.env >/dev/null <<'EOF'
API_ADDR=:8080
AGENT_URL=http://127.0.0.1:8090
MYSQL_DSN=waf:请替换为业务密码@tcp(127.0.0.1:3306)/waf_agent?parseTime=true&charset=utf8mb4
APP_ENCRYPTION_KEY=请替换为32字节以上随机密钥
AUTH_REQUIRED=true
SECURE_COOKIE=false
AUTH_WHITELIST_IPS=127.0.0.1/32,::1/128
EOF
sudo chmod 600 /etc/protein-space/*.env
```

不要把 API Key 写入前端、Git 或日志。启用 HTTPS 后将 `SECURE_COOKIE=true`。

### 6. 配置 systemd

```bash
sudo tee /etc/systemd/system/protein-agent.service >/dev/null <<'EOF'
[Unit]
Description=Protein Space Agent
After=network-online.target
Wants=network-online.target
[Service]
WorkingDirectory=/opt/protein_space
EnvironmentFile=/etc/protein-space/agent.env
ExecStart=/opt/protein_space/bin/agent
Restart=always
RestartSec=3
[Install]
WantedBy=multi-user.target
EOF

sudo tee /etc/systemd/system/protein-api.service >/dev/null <<'EOF'
[Unit]
Description=Protein Space API
After=network-online.target docker.service
Wants=network-online.target
[Service]
WorkingDirectory=/opt/protein_space
EnvironmentFile=/etc/protein-space/api.env
ExecStart=/opt/protein_space/bin/api
Restart=always
RestartSec=3
[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now protein-agent protein-api
sudo systemctl status protein-agent protein-api --no-pager
```

### 7. 配置宿主机 Nginx

```bash
sudo tee /etc/nginx/sites-available/protein-space >/dev/null <<'EOF'
server {
    listen 80;
    server_name _;
    root /opt/protein_space/frontend;
    index index.html;
    location / { try_files $uri $uri/ /index.html; }
    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 1h;
        proxy_send_timeout 1h;
        proxy_set_header Connection "";
    }
}
EOF
sudo ln -sfn /etc/nginx/sites-available/protein-space /etc/nginx/sites-enabled/protein-space
sudo rm -f /etc/nginx/sites-enabled/default
sudo nginx -t
sudo systemctl reload nginx
```

访问 `http://服务器公网IP`。建议使用 Certbot 配置 HTTPS。

### 8. 检查和更新

```bash
curl http://127.0.0.1:8080/api/health
curl http://127.0.0.1:8090/v1/health
curl http://服务器公网IP/api/models
sudo journalctl -u protein-api -f
sudo journalctl -u protein-agent -f
docker compose logs -f mysql
```

更新、重新编译和重启：

```bash
cd /opt/protein_space
git pull --ff-only origin main
(cd api && go mod download && go test ./... && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ../bin/api .)
(cd agent && go test ./... && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ../bin/agent .)
sudo systemctl restart protein-agent protein-api
sudo systemctl reload nginx
```

### 9. MySQL 备份

```bash
set -a; . ./.env; set +a
mkdir -p backups
docker compose exec -T mysql mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" waf_agent | gzip > "backups/waf_agent-$(date +%F-%H%M%S).sql.gz"
```

## 接口和认证

公开接口：`GET /api/health`、`GET /api/models`、`POST /api/auth/register`、`POST /api/auth/login`。

登录后可以使用 HttpOnly Cookie，或者设置：

```http
X-API-Key: waf_xxxxxxxxx
```

主要接口：

```text
GET/POST /api/provider-configs
GET/POST /api/api-keys
GET      /api/conversations
POST     /api/chat/stream
GET/POST /api/auth/whitelist
DELETE   /api/auth/whitelist?id=<id>
```

对话请求示例：

```json
{"conversation_id":"session-001","provider_config_id":1,"model":"gpt-5.5","messages":[{"role":"user","content":"分析这条 WAF 日志"}]}
```

响应为 `text/event-stream`，事件格式为 `data: {"delta":"..."}`，结束事件为 `data: [DONE]`。

## 本地检查

```bash
(cd api && go test ./...)
(cd agent && go test ./...)
node --check frontend/app.js
node --check frontend/auth.js
```
