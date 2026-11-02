# Protein Space / Chat

安装、更新与远程上传的最新入口见 [DEPLOYMENT.md](DEPLOYMENT.md)。安装、更新、远程上传与认证设置统一使用 `scripts/deploy.sh`，执行 `bash scripts/deploy.sh help` 查看用法；内部实现集中在 `scripts/lib/`。

支持 SSE 流式对话、模型选择、用户登录、`X-API-Key`、MySQL 持久化和 IP 白名单。

生产运行结构：

```text
宿主机 Nginx → 宿主机 Go API (:8080) → 宿主机 Go Agent (:8090)
                                      ↓
                             Docker MySQL (127.0.0.1:3306)
```

Docker Compose 只运行 MySQL，不需要拉取 Nginx 或 Go 镜像。Nginx、Go 编译、systemd 和反向代理配置统一由 `scripts/deploy.sh install` 处理。

## 目录

```text
frontend/                 前端页面和 SSE 客户端
wechat-miniprogram/        微信原生小程序（iPhone 微信可用）
ios-app/                  SwiftUI iPhone App（iOS 16+）
api/                      Go API、认证、白名单和 MySQL 持久化
agent/                    Go Agent 和上游模型 SSE
agent/model_protocols.go  模型与上游协议格式映射
agent/protocol_*.go       各上游协议的请求和 SSE 转换
agent/PROTOCOLS.md         模型协议切换说明
mysql/init/               MySQL 初始化表结构
nginx/nginx.conf          Nginx 配置参考
scripts/deploy.sh           安装、更新与远程部署统一入口
relay/                    可选 Python 内网中转代理（支持 SSE）
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
- 保留已有环境文件；首次安装生成 MySQL 随机密码、应用加密密钥及 Agent 配置模板
- 创建并启动 `protein-agent.service`、`protein-api.service`
- 创建宿主机 Nginx 站点和 SSE 反向代理
- 如果已存在 Certbot 证书，同时创建 443 SSL 站点；全部 `/api/` 接口共用 SSE 代理配置

执行：

```bash
sudo bash scripts/deploy.sh install
```

如果已经申请了 `linpro.top` 的证书，脚本会生成 80 和 443 两个监听；443 不做额外 upstream，`/api/` 仍转发到 `127.0.0.1:8080`。如果尚未有证书，先只生成 80 配置，申请证书后再次执行脚本即可启用 443：

```bash
sudo apt install -y certbot python3-certbot-nginx
sudo certbot certonly --nginx -d linpro.top -d www.linpro.top
sudo bash scripts/deploy.sh install
sudo nginx -t && sudo systemctl reload nginx
```

脚本默认配置 `linpro.top`，其他域名可以这样执行：

```bash
sudo DOMAIN=www.linpro.top bash scripts/deploy.sh install
```

如果是第一次执行，脚本会生成数据库密码和配置模板，以退出码 2 提示填写模型配置，不会使用占位符启动服务。保留已生成的密码和加密密钥，填写真实上游配置：

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
sudo bash scripts/deploy.sh install
```

脚本不会覆盖已经存在的环境文件。全新 API、Agent 默认仅监听回环地址。支持自定义项目目录：

```bash
sudo APP_DIR=/opt/protein_space bash scripts/deploy.sh install
```

注意：API 服务读取的是 `/etc/protein-space/api.env`，不是项目根目录 `.env`。根目录 `.env` 主要用于 Docker MySQL；白名单要写入 `api.env` 的 `AUTH_WHITELIST_IPS`，或写入 MySQL 的 `auth_ip_whitelist` 表。

安装脚本只更新本项目的 Nginx 站点和 API snippet，不删除其他站点。已有证书时 HTTP 重定向 HTTPS；发现重复域名配置时需先处理冲突。

### 3. 验证部署

```bash
systemctl status protein-agent protein-api nginx --no-pager
docker compose ps
nginx -t
curl http://127.0.0.1:8090/v1/health
curl http://127.0.0.1:8080/api/health
curl http://服务器公网IP/api/models
```

### 4. 配置 HTTPS 和 443

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

后续只更新代码和服务进程，直接执行：

```bash
cd /opt/protein_space
sudo bash scripts/deploy.sh update
```

更新默认使用当前源码，先测试和编译 API/Agent，再重启服务并校验、应用 Nginx 配置。不会重置数据库卷或环境文件；Git 检出目录若需自动拉取，可使用 `sudo bash scripts/deploy.sh update --pull`。

首次安装执行以下脚本；已有安装也可用 `sudo bash scripts/deploy.sh update nginx` 单独更新上传限制和全部接口配置：

```bash
sudo bash scripts/deploy.sh install
```

之后文件/图片上传相关的代码更新仍然只执行 `deploy.sh update` 即可。

如果需要完整安装或重建系统配置，再执行：

```bash
cd /opt/protein_space
git pull --ff-only origin main
sudo bash scripts/deploy.sh install
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
POST /api/auth/register
POST /api/auth/login
```

登录后可以使用 HttpOnly Cookie，或者设置请求头：

```http
X-API-Key: waf_xxxxxxxxx
```

主要接口（需认证或命中白名单）：

```text
GET      /api/models
GET      /api/conversations/messages?id=<id>
GET/POST /api/admin/config （另需白名单权限）
POST     /api/auth/logout
GET      /api/auth/me
GET/POST /api/api-keys
GET/POST /api/provider-configs
GET      /api/conversations
DELETE   /api/conversations?id=<id>
POST     /api/chat/stream
GET/POST /api/auth/whitelist
DELETE   /api/auth/whitelist?id=<id>
```

历史对话列表支持逐条删除，确认后会永久删除该对话及所有消息。删除接口仅允许操作当前用户的对话；删除当前对话后会打开空白新对话。

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
node --test frontend/app.test.cjs
node --check frontend/auth.js
bash -n scripts/deploy.sh
```

API Key、数据库密码和 `APP_ENCRYPTION_KEY` 不要提交到 Git。之前暴露过的模型 Key 应立即撤销并重新生成。

Agent 只有在 `AGENT_UPSTREAM_URL` 和 `AGENT_API_KEY` 配置正确时才会调用真实模型。未配置时会明确记录 `chat demo` 日志；上游配置存在但调用失败时会返回错误，不再伪装成 GPT 回复。`gpt-5.5` 只是请求的模型名，实际是否支持取决于你的 OpenAI-compatible 服务商。

## 移动端对话客户端

新增两个独立子目录，共用现有后端，支持文字流式对话、模型选择、新建对话、停止生成和服务器历史续聊。

- [微信小程序运行说明](wechat-miniprogram/README.md)：微信开发者工具导入，配置自己的 AppID 和 request 合法域名，iPhone 微信扫码预览。
- [iPhone App 运行说明](ios-app/README.md)：Xcode 打开原生工程，配置开发 Team 后安装到 iPhone。

两端默认连接 `https://www.linpro.top`，当前免登录模式下无需填写 Key，不需要新增后端服务。已有服务器更新后执行 `sudo bash scripts/deploy.sh auth off` 生效；访客聊天只保留当前会话，详见部署说明。

### WAF 规则文档生成

首页或对话页点击「WAF 规则生成」，也可访问 `/waf-rules.html`。粘贴或导入请求日志、HTTP 请求或 PoC 文本，选择模型及可选的已授权 Provider，生成与示例一致的十列表格。支持编辑规则 JSON、下载 HTML 文档、下载接口数据 JSON、复制 cURL。

`POST /api/waf-rules/generate` 接收 `input`、`model`、可选 `context` 和 `provider_config_id`，沿用聊天鉴权、模型权限与 SSE 流。必须配置真实 Agent 上游或选择已授权 Provider；无上游时返回错误，不使用演示回答。生成内容不保存到聊天历史。

接口契约已从 `/Users/linyangzhi/git/wafrule` 的 `app/api_1_0/rule.py`、`rule_utils.py`、`app/views/rule.py`、`app/models/rule.py` 和 `util/record_dict.py` 阅读提取，存于 `api/waf_rules_prompt.txt`，编译时嵌入；部署时不需要原仓库目录。契约随源码变更需同步更新。

目标接口为 `POST /waf_rule/api/create_general`，使用 `application/x-www-form-urlencoded`，需要目标系统登录会话及 `rule_edit` 权限。表单默认观察模式 (`mode=true`)、单次记分 (`fac=false`)。`score_json` 是 JSON 字符串；文档的 `waf_id/name/cve/payload` 和阈值不是创建表单字段。

当前生成普通正则规则草案，不执行 PoC 或提交到目标服务。部署数据库中的阶段/区域映射及严格、普通阈值应填入补充说明；未知值保留待确认，阶段或区域缺失时不生成 cURL。需要跨区域 AND 的链式规则仅显示分析文档并标注待适配，不生成普通规则请求。结构校验不替代目标 WAF 的正则、枚举、误报和效果验证。HTML 可直接用浏览器打开或打印。

验证：`node --test frontend/*.test.cjs`，并分别在 `api/` 和 `agent/` 执行 `go test ./...`。
