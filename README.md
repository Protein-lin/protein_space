# Protein Space / Chat

提供网页聊天、WAF 规则文档生成、微信小程序与 iOS 客户端，共用 Go API 和 Agent。支持 SSE 流式回复、模型选择、登录、个人 API Key、MySQL 历史记录和 IP 白名单。

安装、更新、远程上传统一使用 `scripts/deploy.sh`。本文列出常用命令和配置参数；部署细节见 [DEPLOYMENT.md](DEPLOYMENT.md)，小程序说明见 [wechat-miniprogram/README.md](wechat-miniprogram/README.md)。

## 1. 更新命令速查

服务器命令在项目根目录执行，例如 `/opt/protein_space`。旧版 `install-*.sh`、`update-*.sh` 已合并；首次升级到统一入口先执行 `git pull --ff-only origin main`。

| 更新内容 | 命令 | 实际行为 |
| --- | --- | --- |
| 全站，先拉代码 | `sudo bash scripts/deploy.sh update --pull` | 拉取代码，测试/编译 Agent 和 API，检查 MySQL，重启后端，应用前端/Nginx 配置 |
| 全站，使用当前源码 | `sudo bash scripts/deploy.sh update` | 同上，但不拉代码；适合已经拉取或上传源码 |
| API | `sudo bash scripts/deploy.sh update api --pull` | 拉取整个仓库，只测试、编译并重启 `protein-api` |
| Agent | `sudo bash scripts/deploy.sh update agent --pull` | 拉取整个仓库，只测试、编译并重启 `protein-agent` |
| 网页 | `sudo bash scripts/deploy.sh update frontend --pull` | 拉取整个仓库，使用 `frontend/` 静态文件，校验并应用 Nginx 配置 |
| Nginx | `sudo bash scripts/deploy.sh update nginx` | 重新生成本项目站点和 API snippet，校验并 reload |
| MySQL | `sudo bash scripts/deploy.sh update mysql` | 启动现有 Compose 数据库并检查连接；不自动执行表结构迁移 |
| 微信小程序（开发机） | `bash scripts/deploy.sh update wechat-miniprogram` | 本地测试、语法检查，再打开微信开发者工具；不自动上传或发布 |
| iOS（macOS） | `bash scripts/deploy.sh update ios-app` | 测试并构建模拟器 App；不自动发布 App Store |
| 内网中转机 | `sudo bash scripts/deploy.sh update relay` | 检查本机私有 `relay/proxy.py`，写入并重启中转服务 |

只想更新当前源码时，去掉命令中的 `--pull`。一次只能选择一个组件；多个组件一起更新用 `all`（默认）。`all` 不包含 relay、小程序和 iOS。

本次 WAF 页面涉及 API、Agent 和前端，应执行全站更新：

```bash
cd /opt/protein_space
git pull --ff-only origin main
sudo bash scripts/deploy.sh update
# 更新完成后访问 https://你的域名/waf-rules.html
```

`--pull` 只适用于 `update` 的服务器组件，要求 Git 工作区干净，使用 `git pull --ff-only origin "$BRANCH"`，不会自动切换分支。小程序、iOS、中转机需先自行拉取代码。

前端没有 npm 构建步骤，Nginx 直接读取 `frontend/`。`update frontend` 不重启 API/Agent；后端接口有变化时应全站更新。修改 `api/waf_rules_prompt.txt` 也需要重新编译 API。

## 2. 统一脚本的参数

```bash
bash scripts/deploy.sh help
sudo bash scripts/deploy.sh install                 # 首次安装全站
sudo bash scripts/deploy.sh install api             # 单独安装 API
sudo bash scripts/deploy.sh update api              # 单独更新 API
sudo bash scripts/deploy.sh update --pull           # 拉代码并更新全站
bash scripts/deploy.sh remote my-server update      # 本机上传并更新服务器
sudo bash scripts/deploy.sh auth on                 # 开启聊天认证
sudo bash scripts/deploy.sh auth off                # 允许访客聊天/规则生成
```

`install`/`update` 支持：`all`、`api`、`agent`、`frontend`、`nginx`、`mysql`、`relay`、`ios-app`、`wechat-miniprogram`。远程上传使用 `remote <SSH目标> install|update`，只执行全站安装/更新。省略操作会显示帮助。

### 部署环境变量

以下参数只影响脚本本次运行，不会自动写入 API/Agent 的环境文件。使用 sudo 时将变量写在 `sudo env` 后，避免被 sudo 清除。

| 参数 | 默认值 | 用途与范围 |
| --- | --- | --- |
| `APP_DIR` | 脚本所在仓库根目录 | 服务器源码目录，输出二进制到 `$APP_DIR/bin`；远程部署时由 `REMOTE_DIR` 设置 |
| `PULL` | `0` | `update` 时设为 `1` 等同 `--pull`；客户端和 relay 不支持 |
| `BRANCH` | `main` | `--pull` 拉取的 origin 分支；不执行 checkout |
| `DOMAIN` | `linpro.top` | Nginx 域名；非 `www.` 开头时同时加入 `www.$DOMAIN` |
| `ENV_DIR` | `/etc/protein-space` | 生成/读取 `api.env`、`agent.env`；`auth` 命令也使用此目录 |
| `SYSTEMD_DIR` | `/etc/systemd/system` | `install` 写入 systemd 服务文件的目录 |
| `NGINX_DIR` | `/etc/nginx` | Nginx 站点、启用链接和 snippet 目录 |
| `CERT_DIR` | `/etc/letsencrypt/live/$DOMAIN` | 证书目录，需同时存在 `fullchain.pem`、`privkey.pem` 才启用 HTTPS |
| `LOCK_FILE` | `/var/lock/protein-space-deploy.lock` | 服务器部署锁，避免并发更新 |
| `REMOTE_DIR` | `/opt/protein_space` | 仅用于 `remote`：服务器接收源码的目录 |

服务器目录参数要求无空格的绝对路径。更改 `APP_DIR`、`ENV_DIR` 或 systemd 路径后，需要执行 `install` 重建服务定义；单纯 `update` 不重写 unit。API/Agent 监听端口、Agent 地址和 Nginx 转发地址需保持一致，具体见后文。

```bash
# 自定义目录和域名，更新 API
sudo env APP_DIR=/opt/protein_space DOMAIN=chat.example.com \
  bash scripts/deploy.sh update api --pull

# 自定义证书目录，更新网页/Nginx
sudo env DOMAIN=chat.example.com CERT_DIR=/etc/letsencrypt/live/chat.example.com \
  bash scripts/deploy.sh update frontend

# 从本机上传源码；SSH 端口、IdentityFile 等放在 ~/.ssh/config 的 my-server 中
REMOTE_DIR=/opt/protein_space DOMAIN=chat.example.com \
  bash scripts/deploy.sh remote my-server update
```

远程上传打包当前工作区，不要求先提交 Git；包含新 WAF 页面的提示词文件，排除 `.env`、Git、构建产物、私有配置和 `relay/proxy.py`。远端先备份源码，再覆盖并执行统一入口，强制 `PULL=0`。当前只传递 `REMOTE_DIR` 对应的 `APP_DIR` 和 `DOMAIN`，不会透传本机其他环境变量。SSH 需可信 known_hosts；非 root 账号需免密 sudo。

## 3. 首次安装和服务配置

运行结构：

```text
Nginx → Go API (127.0.0.1:8080) → Go Agent (127.0.0.1:8090)
                ↓
       Docker MySQL (127.0.0.1:3306)
```

服务器使用 Ubuntu/Debian + systemd；Docker Compose 仅运行 MySQL。公网入口为 80/443，API、Agent、数据库保持内网监听。

```bash
sudo git clone git@github.com:Protein-lin/protein_space.git /opt/protein_space
sudo chown -R "$USER":"$USER" /opt/protein_space
cd /opt/protein_space
sudo bash scripts/deploy.sh install
```

首次安装生成随机数据库密码、应用加密密钥及模型配置模板。发现占位符时退出码为 `2`；编辑配置后重新执行相同命令。已有配置不会覆盖，更新不会删除数据库卷。API/Agent 在编译成功后才替换二进制，服务重启或健康检查失败时尝试恢复上一版二进制；整个全站更新不是事务。

### API：`/etc/protein-space/api.env`

| 参数 | 安装脚本生成值 | 说明 |
| --- | --- | --- |
| `API_ADDR` | `127.0.0.1:8080` | API 监听地址；直接运行 Go 程序、未设置时默认 `:8080` |
| `AGENT_URL` | `http://127.0.0.1:8090` | 内部 Agent 地址，不加 `/v1`；程序默认 `http://localhost:8090` |
| `MYSQL_DSN` | `waf:<生成的密码>@tcp(127.0.0.1:3306)/waf_agent?parseTime=true&charset=utf8mb4` | 数据库连接；未配置时禁用数据库持久化 |
| `APP_ENCRYPTION_KEY` | 随机 32 字节的十六进制字符串 | 加密 Provider Key；应保留原值，否则历史密文无法解密。程序内置的开发兜底值不用于生产 |
| `AUTH_REQUIRED` | `false` | `true`/`1` 要求聊天认证；`false` 允许访客模型查询、聊天及规则生成 |
| `AUTH_WHITELIST_IPS` | `127.0.0.1/32,::1/128` | 逗号分隔的 IP/CIDR；数据库白名单之外的环境配置，程序未设置时为空 |

```bash
sudo vi /etc/protein-space/api.env
# 仅改环境配置时，重启即可加载
sudo systemctl restart protein-api
# 修改 API 代码时，测试并编译后更新
sudo bash scripts/deploy.sh update api
```

`api.env` 由 systemd 加载，项目根目录 `.env` 不会自动成为 API 配置。直接 `go run` 时需自行设置环境变量。登录 Cookie 使用 Secure 属性，浏览器账号登录应通过 HTTPS。

当前脚本的健康检查固定使用 API `8080`、Agent `8090`，Nginx snippet 也固定代理到 `127.0.0.1:8080`。自定义监听端口时还需同步修改 `scripts/lib/server.sh` 的健康检查参数和 `nginx/api-locations.conf`，不能只改 `API_ADDR`/`AGENT_ADDR`。

### Agent：`/etc/protein-space/agent.env`

| 参数 | 安装脚本生成值 | 说明 |
| --- | --- | --- |
| `AGENT_ADDR` | `127.0.0.1:8090` | 内部监听地址；程序未设置时默认 `:8090` |
| `AGENT_UPSTREAM_URL` | `https://REPLACE_ME/v1/chat/completions` | 必须替换为真实模型服务地址；支持根地址或协议端点，详见 [协议说明](agent/PROTOCOLS.md) |
| `AGENT_API_KEY` | `REPLACE_ME` | 模型服务商 Key，仅放在服务端 |
| `AGENT_MODEL` | `gpt-5.6-terra` | 默认模型 ID，应改为上游实际支持且模型列表可用的 ID |

```bash
sudo vi /etc/protein-space/agent.env
sudo systemctl restart protein-agent
# 修改模型协议映射或 Agent 代码时
sudo bash scripts/deploy.sh update agent
```

未设置上游时，普通聊天会返回演示回复；WAF 规则生成会明确报错，必须配置真实上游或选择已授权 Provider。也可在网页「系统配置 → Agent / Provider」填写 `name`、`base_url`、`api_key`、`default_model`；需要白名单权限。请求中的 `provider_config_id` 必须属于当前用户，浏览器传入的 `upstream_url`、`api_key` 不会用于选择上游。

### MySQL：项目根目录 `.env`

| 参数 | 默认/来源 | 说明 |
| --- | --- | --- |
| `MYSQL_ROOT_PASSWORD` | 安装生成随机值 | MySQL root 密码，Compose 必填 |
| `MYSQL_PASSWORD` | 安装生成随机值 | 业务用户 `waf` 的密码，需与 API DSN 一致 |

数据库名 `waf_agent`、用户 `waf`、端口 `127.0.0.1:3306` 和卷 `mysql_data` 定义在 `docker-compose.yml`。初始化 SQL 仅在空数据卷首次启动时执行；已有数据库修改密码或表结构需单独处理，修改 `.env` 不会自动修改库内密码。

## 4. 网页、Nginx 和 WAF 页面

| 位置/参数 | 默认/行为 | 修改后如何生效 |
| --- | --- | --- |
| 首页与聊天 | `/`，源码 `frontend/index.html`、`app.js` | 更新 `frontend/`，执行 `update frontend`；浏览器刷新 |
| WAF 工作台 | `/waf-rules.html` | 前端改动用 `update frontend`；API/Agent 同时变化用全站更新 |
| 网页 API 地址 | 同源 `/api/`，没有单独的前端 API 域名环境变量 | 由 Nginx 代理；如需跨站需改前端实现和认证配置 |
| 登录/个人 Key | HttpOnly Cookie；兼容浏览器 localStorage 的 `waf_api_key`，自动发送 `X-API-Key` | 个人 Key 与模型服务商 Key 不同；普通网页提供登录入口，没有单独的 Key 输入表单 |
| 模型列表 | `/api/models` 返回 `models` 与 `default`；网页优先选择本浏览器上次请求使用的可用模型 | 服务端上游与用户模型权限决定；恢复历史对话时保留该对话模型 |
| 聊天附件 | 最多 3 个，每个最多 5 MiB，图片或支持的文本文件 | 限制在 `frontend/app.js`；请求还受 Nginx/API 的 24 MiB 限制 |
| Nginx 站点 | `/etc/nginx/sites-available/protein-space` | 用 `DOMAIN`、`CERT_DIR` 重新运行 `update nginx` |
| API snippet | `/etc/nginx/snippets/protein-space-api.conf` | 源码 `nginx/api-locations.conf`；全站或 frontend/nginx 更新时覆盖 |
| SSE 代理 | 关闭缓冲、缓存及 gzip，读写超时 `1h` | 修改 snippet 源码后更新 nginx |
| HTTPS | 找到对应证书文件时启用，HTTP 跳转 HTTPS | 脚本不申请证书；准备证书后执行 `update nginx` |

默认 `DOMAIN=linpro.top` 同时配置 `linpro.top` 和 `www.linpro.top`，证书默认从 `/etc/letsencrypt/live/linpro.top` 读取。访问 www 地址不需要把 DOMAIN 改成 www，否则证书查找目录也会改变。更换域名应在后续更新时继续传入相同的 DOMAIN；脚本不会自动记住上次值。

网页聊天和 WAF 页面在发起请求时，将模型 ID 保存到当前站点 localStorage 的 `protein_last_requested_model`。仅切换下拉框不会写入；请求失败也保留本次选择。该偏好限当前浏览器和站点，清除浏览器数据后恢复默认，不改变其他用户或小程序的模型设置。

### WAF 页面输入参数

| 字段 | 默认/限制 | 用途 |
| --- | --- | --- |
| 原始内容 | 必填；文本最多 180 KiB，输入框最多 180000 字符 | HTTP 请求、请求日志或 PoC，只分析文本、不执行代码 |
| 导入文件 | 单个最多 180 KiB | `.txt/.log/.json/.md/.yaml/.yml/.py/.http/.xml/.csv` |
| 模型服务 | 默认 Agent，或当前用户可用 Provider | 选择后发送对应 `provider_config_id` |
| Agent 模型 | 优先使用本浏览器上次请求的模型，不可用时回退到服务端默认/第一个可用模型 | 必填；需真实模型服务；与网页聊天共享记录 |
| 目标 WAF 地址 | `https://waf.example.com` | 必须改为目标系统的 http(s) 主机地址，可带端口，不带路径、凭据或查询参数；只用于生成请求 |
| 部署配置/补充说明 | 选填，最多 24000 字符 | 提供数据库中 phase/zone 关系、计分类别、严格/普通阈值 |
| 规则 JSON | 生成后可编辑并「应用修改」 | 结构校验通过后更新表格和请求；最多 30 条规则 |

输出列为 `waf_id/name/cve/type/payload/ackey/rule/regex/zone/score`，分数顺序为「设置分数 / 严格阈值 / 普通阈值」。支持 HTML 文档和接口数据 JSON 导出，以及完整普通规则草案的 cURL 复制。WAF ID 由目标系统分配，未知部署值保留待确认；缺 phase/zone 时不生成 cURL，需要链式条件时仅给分析说明。

契约从本机 `/Users/linyangzhi/git/wafrule` 的规则创建、校验、区域映射和模型代码提取，保存在 `api/waf_rules_prompt.txt` 并编译嵌入 API。部署时不读取该本机目录；目标源码变化需同步契约并重新编译。

生成的目标请求为 `POST /waf_rule/api/create_general`，使用 `application/x-www-form-urlencoded`：`ackey` 以换行分隔，`phase/zone` 以逗号分隔，`score_json` 是 JSON 字符串。默认 `operator=rx`、`mode=true`（观察）、`fac=false`（仅记一次分）、`base64_decode=false`、`usetype=default`。name/cve/payload/阈值是文档字段，不进入表单。目标需要有效登录 Cookie 和 `rule_edit` 权限，生成的 cURL 使用本机 `cookies.txt`；页面不自动登录、提交或发布规则。

## 5. 微信小程序更新和参数

以下命令在开发机的仓库根目录执行，不需要 sudo：

```bash
git pull --ff-only origin main
bash scripts/deploy.sh update wechat-miniprogram

# 只跑本地测试，不打开开发者工具，不连接线上服务
CHECK_ONLY=1 bash scripts/deploy.sh update wechat-miniprogram

# 自定义微信开发者工具 CLI 路径
WECHAT_CLI=/Applications/wechatwebdevtools.app/Contents/MacOS/cli \
  bash scripts/deploy.sh update wechat-miniprogram
```

`install wechat-miniprogram` 与 `update wechat-miniprogram` 行为相同，均不拉代码、不更新服务器，也不上传、预览或发布小程序。需要 Node.js 18+；打开工具时要求 CLI 文件存在且可执行。

| 参数 | 位置 | 默认/说明 |
| --- | --- | --- |
| `CHECK_ONLY` | 运行脚本时的环境变量 | 默认 `0`；设为 `1` 只测试 |
| `WECHAT_CLI` | 运行脚本时的环境变量 | 默认 `/Applications/wechatwebdevtools.app/Contents/MacOS/cli`；支持带空格路径，赋值时加引号 |
| `appid` | `wechat-miniprogram/project.config.json` | 仓库默认 `touristappid`；使用自己的 AppID，本机修改值不要覆盖成示例值 |
| `miniprogramRoot` | 同上 | 仓库默认空字符串，以该配置文件所在目录为小程序根目录；开发工具导入 `wechat-miniprogram/` |
| `compileType` | 同上 | `miniprogram` |
| `projectname` | 同上 | `protein-space-chat`，开发工具项目名称 |
| `libVersion` | 同上 | 仓库声明 `3.4.9`，是项目基础库设置，并非“最新版本”声明 |
| `setting.urlCheck` | 同上 | 仓库默认 `true`，保留域名校验 |
| `setting.es6` / `setting.minified` | 同上 | 仓库默认 `true` / `true`，启用 ES6 与压缩 |
| 服务地址 `base` | 小程序页面「设置」 | 默认 `https://www.linpro.top`；必须为 HTTPS 主机地址，可带端口，不要加 `/api` 或路径 |
| 个人 Key `key` | 小程序页面「设置」 | 默认空；需要认证时填写本系统个人 API Key，不是模型服务商 Key |
| 当前模型 | 页面模型选择器 | 使用 `/api/models` 返回的 default，找不到时选第一个；请求使用对应模型 ID |
| 普通请求超时 | `wechat-miniprogram/utils/api.js` | `20000` 毫秒，代码常量，不是脚本环境参数 |
| 流式请求超时 | 同上 | `600000` 毫秒，使用 `enableChunked: true` 与 `responseType: 'arraybuffer'` |

连接成功后，地址保存到本地存储键 `protein-base`；个人 Key 仅留在页面内存，重启后需重新输入。旧默认地址 `https://linpro.top` 会迁移到 www，自定义地址保留。更改地址或 Key 后需点「连接服务」，会清空当前对话以重新连接。

小程序后台的 request 合法域名应与页面地址一致，例如 `https://www.linpro.top`，使用有效 HTTPS 证书。代码检查通过后，在开发者工具中完成编译、真机预览和发布流程。当前小程序只有文字聊天、模型选择、停止生成和历史续聊，没有图片上传或 WAF 规则工作台。

`AUTH_REQUIRED=false` 时可留空 Key 聊天，但访客不保存账号历史。读取和续聊持久化历史需要有效个人 Key（或白名单身份）及后端 MySQL。填写 Key 时请求才携带 `X-API-Key`。

### 可选线上小程序联调

此测试会真实调用模型，创建并删除它自己的 `mobile-smoke-*` 测试会话；需要可保存历史的身份、MySQL 和真实模型服务，不包含在普通 `update`/`CHECK_ONLY` 中。

| 参数 | 默认 | 说明 |
| --- | --- | --- |
| `PROTEIN_TEST_BASE` | 测试代码默认 `https://linpro.top` | 建议显式设为实际 HTTPS 站点，与客户端地址保持一致 |
| `PROTEIN_TEST_KEY` | 空 | 本系统个人 API Key，需能访问模型和历史 |

```bash
# 先在本地环境中设置 PROTEIN_TEST_KEY，再执行；不要把真实 Key 写入仓库
PROTEIN_TEST_BASE=https://www.linpro.top node wechat-miniprogram/tests/live.cjs
```

## 6. API 请求与认证参数

`GET /api/health`、注册和登录接口公开。其余接口受身份和权限检查；`AUTH_REQUIRED=false` 仅开放访客模型查询、聊天、规则生成及访客身份/空历史列表，不开放后台管理。白名单可配置于 `AUTH_WHITELIST_IPS` 或数据库 `auth_ip_whitelist`；管理页需白名单权限。API 应通过 Nginx 对外，避免客户端直接伪造代理头。

| 请求 | 用途 |
| --- | --- |
| `GET /api/models` | 可用模型列表与默认模型 |
| `POST /api/chat/stream` | 流式聊天 |
| `POST /api/waf-rules/generate` | WAF 规则文档生成 |
| `POST /api/auth/register`、`POST /api/auth/login` | 注册/登录：username、password，注册可选 email |
| `GET /api/auth/me`、`POST /api/auth/logout` | 当前身份/退出 |
| `GET/POST /api/api-keys` | 个人 Key 查询/创建 |
| `GET/POST /api/provider-configs` | 当前用户 Provider 查询/创建 |
| `GET /api/conversations` | 当前账号历史列表，访客为空 |
| `GET /api/conversations/messages?id=<id>` | 指定会话消息，要求会话归属当前用户 |
| `DELETE /api/conversations?id=<id>` | 永久删除当前用户的指定会话及消息 |
| `GET/POST /api/admin/config` | 管理配置，需要白名单权限 |
| `GET/POST/DELETE /api/auth/whitelist` | 白名单管理；DELETE 使用 `?id=<id>` |

个人 Key 可在已登录 Cookie 会话下调用 `POST /api/api-keys` 创建：`name` 为必填名称，`expires_at` 可省略或为 null，也可填写 RFC3339 时间（例如 `2027-01-01T00:00:00Z`）。响应的 `key` 是完整 Key，列表接口只返回前缀，创建后需自行保存。小程序设置中填写这个值。

```bash
# cookies.txt 需为本系统已登录会话导出的 Cookie 文件
curl --fail --cookie cookies.txt \
  --header 'Content-Type: application/json' \
  --data '{"name":"wechat-client","expires_at":null}' \
  https://www.linpro.top/api/api-keys
```

聊天请求 `Content-Type: application/json`，通过登录 Cookie 或 `X-API-Key: waf_...` 认证：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `model` | string，必填 | `/api/models` 返回的可用 ID，服务端检查权限 |
| `messages` | 非空数组，必填 | 每项含 role/content；文字 content 为字符串，网页图片使用内容数组；调用方发送完整上下文 |
| `conversation_id` | string，可选 | 同一对话沿用 ID；有有效身份、数据库和非空 ID 时用于持久化 |
| `provider_config_id` | integer，可选 | 省略或 `0` 使用默认 Agent 上游；非零值必须是当前用户被授权的 Provider |

```json
{
  "conversation_id": "session-001",
  "model": "替换为模型列表中的ID",
  "messages": [{"role": "user", "content": "分析这条 WAF 日志"}]
}
```

WAF 生成接口改用以下字段；请求体上限 256 KiB，不保存到聊天历史：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `input` | string，必填 | 日志或 PoC 原文 |
| `model` | string，必填 | 可用模型 ID |
| `context` | string，可选 | 部署配置与补充说明 |
| `provider_config_id` | integer，可选 | 同聊天接口 |

两类接口响应均为 `text/event-stream`，事件形如 `data: {"delta":"..."}`，错误可为 `data: {"error":"..."}`，结束为 `data: [DONE]`。规则生成的 delta 拼接后为规则文档 JSON。目标 WAF 地址只在浏览器生成请求时使用，不是此生成接口的参数。

## 7. 检查、日志与其他客户端

```bash
# 服务器健康检查
sudo nginx -t
systemctl is-active protein-agent protein-api nginx
curl --fail http://127.0.0.1:8090/v1/health
curl --fail http://127.0.0.1:8080/api/health
curl --fail https://www.linpro.top/api/health

# 查看日志
journalctl -u protein-api -f
journalctl -u protein-agent -f
docker compose logs -f mysql

# 本地代码检查（不会发起真实模型调用）
(cd api && go test ./...)
(cd agent && go test ./...)
node --test frontend/*.test.cjs
CHECK_ONLY=1 bash scripts/deploy.sh update wechat-miniprogram
python3 -m unittest discover -s scripts/tests -p 'test_*.py'
bash -n scripts/deploy.sh
```

| 现象 | 检查方向 |
| --- | --- |
| 安装退出码 2 | 填写生成的环境文件占位符，再执行 install |
| `--pull` 提示本地改动 | 检查 `git status`，先处理改动，不强制覆盖 |
| API/Agent 更新失败 | 查看测试或 journalctl 输出；核对环境文件和固定健康检查端口 |
| 网页仍是旧版本 | 确认拉取代码、Nginx root 指向正确 APP_DIR，再强制刷新浏览器 |
| WAF 页面提示需要真实服务 | 配置 Agent 上游或选择已授权 Provider；仅更新前端不够 |
| 小程序 HTTP 401/403 | 检查个人 Key、AUTH_REQUIRED、账号与模型权限 |
| 小程序无历史 | 访客无持久化历史；检查个人 Key 和 MySQL |
| 小程序连接失败 | 检查实际 HTTPS 地址、request 合法域名和证书，地址不能带 `/api` |

其他客户端参数：`BUILD_DIR` 默认 `ios-app/build`，用于 iOS 测试和构建输出；`SIMULATOR_ID` 可选，用于构建后安装到指定模拟器。完整 Xcode、真机签名等见 [iOS 说明](ios-app/README.md)。relay 需预先在内网机器配置私有 `relay/proxy.py`，不在全站更新或远程部署包内。

目录：`frontend/` 网页，`api/` API 服务，`agent/` 模型代理，`mysql/init/` 初始表结构，`nginx/` 代理配置，`scripts/lib/` 部署内部实现，`wechat-miniprogram/` 微信客户端，`ios-app/` 原生 iOS 客户端。
