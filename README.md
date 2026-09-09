# Protein Space / Chat

支持 SSE 流式对话、模型选择、用户登录、`X-API-Key`、MySQL 持久化和 IP 白名单。

生产运行结构：

```text
宿主机 Nginx → 宿主机 Go API (:8080) → 宿主机 Go Agent (:8090)
                                      ↓
                             Docker MySQL (127.0.0.1:3306)
```

Docker Compose 只运行 MySQL，不需要拉取 Nginx 或 Go 镜像。Nginx、Go 编译、systemd 和反向代理配置统一由 `scripts/install-host.sh` 处理。

## 目录

```text
frontend/                 前端页面和 SSE 客户端
api/                      Go API、认证、白名单和 MySQL 持久化
agent/                    Go Agent 和上游模型 SSE
mysql/init/               MySQL 初始化表结构
nginx/nginx.conf          Nginx 配置参考
scripts/install-host.sh   Ubuntu 宿主机一键安装脚本
docker-compose.yml        仅启动 MySQL
```

## Ubuntu 24.04 / 阿里云 ECS 部署

以下命令在服务器 `/opt/protein_space` 执行，示例域名为 `linpro.top`。安全组开放 `80/tcp` 和 `443/tcp`；不要把 `3306`、`8080`、`8090` 暴露到公网。

### 1. 拉取代码

```bash
sudo mkdir -p /opt
sudo git clone git@github.com:Protein-lin/protein_space.git /opt/protein_space
sudo chown -R "$USER":"$USER" /opt/protein_space
cd /opt/protein_space
```

已有项目直接更新：

```bash
cd /opt/protein_space
git pull --ff-only origin main
```

### 2. 执行一键安装脚本

脚本会自动完成：

- 安装 Nginx、Go、Docker 和 Compose
- 启动 Docker 和 Nginx
- 启动仅监听本机的 MySQL
- 编译 API 和 Agent 为 Linux 二进制
- 创建 `/etc/protein-space/agent.env` 和 `api.env` 模板
- 创建并启动 `protein-agent.service`、`protein-api.service`
- 创建宿主机 Nginx 站点和 SSE 反向代理
- 如果已存在 Certbot 证书，同时创建 443 SSL 站点

执行：

```bash
sudo bash scripts/install-host.sh
```

如果已经申请了 `linpro.top` 的证书，脚本会生成 80 和 443 两个监听；443 不做额外 upstream，`/api/` 仍转发到 `127.0.0.1:8080`。如果尚未有证书，先只生成 80 配置，申请证书后再次执行脚本即可启用 443：

```bash
sudo apt install -y certbot python3-certbot-nginx
sudo certbot certonly --nginx -d linpro.top -d www.linpro.top
sudo bash scripts/install-host.sh
sudo nginx -t && sudo systemctl reload nginx
```

脚本默认配置 `linpro.top`，其他域名可以这样执行：

```bash
sudo DOMAIN=www.linpro.top bash scripts/install-host.sh
```

如果是第一次执行，脚本会创建配置模板后退出，不会使用占位符启动服务。填写真实配置：

```bash
vi /opt/protein_space/.env
sudo vi /etc/protein-space/agent.env
sudo vi /etc/protein-space/api.env
sudo chmod 600 /etc/protein-space/*.env
```

至少需要修改：

```env
# agent.env
AGENT_UPSTREAM_URL=https://你的模型服务地址/v1/chat/completions
AGENT_API_KEY=新的模型服务Key

# api.env
MYSQL_DSN=waf:MySQL业务密码@tcp(127.0.0.1:3306)/waf_agent?parseTime=true&charset=utf8mb4
APP_ENCRYPTION_KEY=32字节以上随机密钥
```

再次执行脚本完成编译、服务和 Nginx 配置：

```bash
sudo bash scripts/install-host.sh
```

脚本不会覆盖已经存在的环境文件。支持自定义项目目录：

```bash
sudo APP_DIR=/opt/protein_space bash scripts/install-host.sh
```

注意：API 服务读取的是 `/etc/protein-space/api.env`，不是项目根目录 `.env`。根目录 `.env` 主要用于 Docker MySQL；白名单要写入 `api.env` 的 `AUTH_WHITELIST_IPS`，或写入 MySQL 的 `auth_ip_whitelist` 表。

安装脚本会移除 `/etc/nginx/sites-enabled/` 中仍引用旧域名 `tranquilsoulspace.top` 或旧 upstream `127.0.0.1:3000` 的站点链接，避免旧配置导致 502 或 80 端口 301 跳转；项目配置的 80 端口不做跳转。

### 3. 验证部署

```bash
systemctl status protein-agent protein-api nginx --no-pager
docker compose ps
nginx -t
curl http://127.0.0.1:8090/v1/health
curl http://127.0.0.1:8080/api/health
curl http://服务器公网IP/api/models
```

### 3. 配置 HTTPS 和 443

先确认 DNS 的 `A` 记录已经指向服务器公网 IP，再执行：

```bash
sudo apt install -y certbot python3-certbot-nginx
sudo certbot --nginx -d linpro.top -d www.linpro.top
```

Certbot 会生成 443 SSL 配置并把 80 重定向到 443；原有 `/api/` 反向代理会继续转发到 `127.0.0.1:8080`。验证：

```bash
sudo nginx -t
sudo systemctl reload nginx
curl -vk https://linpro.top/api/health
```

如果只使用主域名：

```bash
sudo certbot --nginx -d linpro.top
```

浏览器访问：

```text
https://linpro.top
```

服务日志：

```bash
journalctl -u protein-agent -f
journalctl -u protein-api -f
tail -f /var/log/nginx/error.log
docker compose logs -f mysql
```

### 4. 更新和重新编译

```bash
cd /opt/protein_space
git pull --ff-only origin main
sudo bash scripts/install-host.sh
sudo systemctl restart protein-agent protein-api
sudo systemctl reload nginx
```

脚本会重新编译：

```text
/opt/protein_space/bin/api
/opt/protein_space/bin/agent
```

如果是 ARM 服务器，脚本会直接在服务器本地编译；查看架构：

```bash
uname -m
file /opt/protein_space/bin/api /opt/protein_space/bin/agent
```

### 5. MySQL 初始化和备份

首次启动自动执行 `mysql/init/001_schema.sql`。已有数据卷不会重复执行初始化脚本。更新表结构时：

```bash
set -a; . ./.env; set +a
docker compose exec -T mysql mysql -uroot -p"$MYSQL_ROOT_PASSWORD" waf_agent < mysql/init/001_schema.sql
```

备份：

```bash
set -a; . ./.env; set +a
mkdir -p backups
docker compose exec -T mysql mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" waf_agent | gzip > "backups/waf_agent-$(date +%F-%H%M%S).sql.gz"
```

## 认证和接口

公开接口：

```text
GET  /api/health
GET  /api/models
POST /api/auth/register
POST /api/auth/login
```

登录后可以使用 HttpOnly Cookie，或者设置请求头：

```http
X-API-Key: waf_xxxxxxxxx
```

主要接口：

```text
POST     /api/auth/logout
GET      /api/auth/me
GET/POST /api/api-keys
GET/POST /api/provider-configs
GET      /api/conversations
POST     /api/chat/stream
GET/POST /api/auth/whitelist
DELETE   /api/auth/whitelist?id=<id>
GET      /v1/health       (Agent 内部接口)
```

IP 白名单命中后可以免登录和免 `X-API-Key`。白名单支持单 IP 或 CIDR，数据库表为 `auth_ip_whitelist`，环境变量 `AUTH_WHITELIST_IPS` 可配置逗号分隔的兜底范围。API 只应通过 Nginx 对外提供，避免客户端伪造 `X-Real-IP`。

对话请求示例：

```json
{
  "conversation_id": "session-001",
  "provider_config_id": 1,
  "model": "gpt-5.5",
  "messages": [{"role": "user", "content": "分析这条 WAF 日志"}]
}
```

响应类型为 `text/event-stream`，事件格式为 `data: {"delta":"..."}`，结束事件为 `data: [DONE]`。

## 本地开发检查

```bash
(cd api && go test ./...)
(cd agent && go test ./...)
node --check frontend/app.js
node --check frontend/auth.js
bash -n scripts/install-host.sh
```

API Key、数据库密码和 `APP_ENCRYPTION_KEY` 不要提交到 Git。之前暴露过的模型 Key 应立即撤销并重新生成。

Agent 只有在 `AGENT_UPSTREAM_URL` 和 `AGENT_API_KEY` 配置正确时才会调用真实模型。未配置时会明确记录 `chat demo` 日志；上游配置存在但调用失败时会返回错误，不再伪装成 GPT 回复。`gpt-5.5` 只是请求的模型名，实际是否支持取决于你的 OpenAI-compatible 服务商。
