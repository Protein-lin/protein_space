# WAF Agent 四项目结构

仓库内包含四个子项目：

- `frontend`：参考截图的对话页面，使用原生 HTML/CSS/JS，通过 `POST /api/chat/stream` 消费 SSE。
- `api`：Go API 网关，统一鉴权/CORS/请求校验，并将 SSE 转发给 Agent。
- `agent`：Go Agent 服务，支持模型选择；配置 `AGENT_UPSTREAM_URL` 和 `AGENT_API_KEY` 后可代理 OpenAI 兼容的流式接口，未配置时使用可测试的演示流。
- `nginx`：静态页面托管和 `/api` 反向代理，已关闭 SSE 缓冲。
- `mysql`：MySQL 8.0 初始化脚本，保存用户、模型服务配置、会话、消息和调用统计。

API 支持两种身份校验方式：浏览器登录后使用 `waf_session` HttpOnly Cookie，或在服务调用时设置 `X-API-Key` 请求头。模型服务配置按用户授权隔离，API Key 在数据库中使用 `APP_ENCRYPTION_KEY` 加密保存。

认证还支持 IP 白名单：命中 `auth_ip_whitelist` 表中的 IP/CIDR，或命中环境变量 `AUTH_WHITELIST_IPS`（逗号分隔）时，不需要登录或 `X-API-Key`。Nginx 会把客户端 IP 通过 `X-Real-IP` 转给 API；不要在 API 端口直接暴露公网并让客户端自行伪造这些 Header。

## 启动

```bash
docker compose up --build
# 浏览器访问 http://localhost:8088
```

首次启动会自动创建 MySQL 数据库和表。生产环境请通过 `.env` 修改 `MYSQL_ROOT_PASSWORD`、`MYSQL_PASSWORD` 和 `APP_ENCRYPTION_KEY`。

首次使用可调用 `POST /api/auth/register` 注册；登录接口为 `POST /api/auth/login`。登录后可通过 `POST /api/api-keys` 创建个人 API Key，通过 `POST /api/provider-configs` 添加自己的模型服务地址和 Key。对话请求可传 `provider_config_id` 和 `model`，API 会校验当前用户权限后路由到对应 Agent。

白名单管理接口：`GET/POST /api/auth/whitelist`，删除使用 `DELETE /api/auth/whitelist?id=<id>`。数据库初始化默认加入 `127.0.0.1/32` 和 `::1/128`，公网网段请谨慎添加。

## Ubuntu 24.04 LTS 服务器部署

以下命令适用于 Ubuntu 24.04.1 LTS，使用 Docker Compose 运行 MySQL、Agent、API 和 Nginx。

### 1. 安装 Docker Engine 和 Compose

```bash
sudo apt update
sudo apt install -y ca-certificates curl gnupg
sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
sudo chmod a+r /etc/apt/keyrings/docker.gpg

echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu noble stable" | \
  sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

sudo apt update
sudo apt install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
sudo systemctl enable --now docker
sudo usermod -aG docker "$USER"
```

重新登录服务器后检查：

```bash
docker --version
docker compose version
```

### 2. 部署项目

```bash
git clone <你的项目仓库地址> protein_space
cd protein_space
```

创建生产环境配置文件：

```bash
cat > .env <<'EOF'
MYSQL_ROOT_PASSWORD=请替换为强随机密码
MYSQL_PASSWORD=请替换为强随机密码
APP_ENCRYPTION_KEY=请替换为32字节以上的随机密钥
AGENT_MODEL=gpt-5.5
AGENT_UPSTREAM_URL=https://你的模型服务地址/v1/chat/completions
AGENT_API_KEY=请替换为新的模型服务Key
AUTH_WHITELIST_IPS=127.0.0.1/32,10.0.0.0/8
EOF
chmod 600 .env
```

启动全部服务：

```bash
docker compose up -d --build
docker compose ps
```

页面默认访问地址：

```text
http://服务器公网IP:8088
```

如果使用 UFW，可以开放 HTTP 端口：

```bash
sudo ufw allow OpenSSH
sudo ufw allow 8088/tcp
sudo ufw enable
```

### 3. 验证服务

```bash
curl http://127.0.0.1:8088/api/health
curl http://127.0.0.1:8088/api/models
docker compose logs -f nginx
docker compose logs -f api
docker compose logs -f agent
```

### 4. MySQL 常用命令

```bash
set -a; . ./.env; set +a

# 查看 MySQL 容器状态
docker compose exec mysql mysqladmin ping -uwaf -p"$MYSQL_PASSWORD"

# 进入数据库
docker compose exec mysql mysql -uwaf -p"$MYSQL_PASSWORD" waf_agent

# 备份数据库
mkdir -p backups
docker compose exec -T mysql mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" waf_agent \
  | gzip > "backups/waf_agent-$(date +%F-%H%M%S).sql.gz"
```

### 5. Nginx 和 SSE 注意事项

项目内的 `nginx/nginx.conf` 已配置 SSE 所需的 `proxy_buffering off` 和长连接超时。生产环境建议在 Nginx 前再接 HTTPS，并将 8088 映射为 80/443；不要把 MySQL 3306 端口暴露到公网。

更新代码后执行：

```bash
git pull
docker compose up -d --build
docker image prune -f
```

不使用 Docker 时分别启动：

```bash
(cd agent && go run .)
(cd api && AGENT_URL=http://localhost:8090 go run .)
python3 -m http.server 3000 --directory frontend
```

开发环境下把前端请求代理到 `localhost:8080`，或者直接用 Nginx 配置运行。模型切换通过页面左下角下拉框完成，模型会随每次请求的 `model` 字段传到 Agent。

## 接口约定

`POST /api/chat/stream` 请求体：

```json
{"conversation_id":"...","model":"gpt-5.5","messages":[{"role":"user","content":"查询规则"}]}
```

响应为 `text/event-stream`，每个事件形如 `data: {"delta":"..."}`，结束事件为 `data: [DONE]`。
