# 安装、更新与上传服务器

服务器：Ubuntu 24.04 / Debian（Go 1.22+、systemd）。公网入口为 Nginx，API 和 Agent 新安装时只监听 127.0.0.1。微信和 iOS 共用此后端，不需要新增公网服务。

## 从本机上传并安装

```bash
bash scripts/deploy.sh root@服务器IP install
# 使用 SSH 配置里的别名（包括非标准端口）
bash scripts/deploy.sh my-server update
# 自定义目录与域名
REMOTE_DIR=/opt/protein_space DOMAIN=chat.example.com bash scripts/deploy.sh my-server install
```

需要配置 SSH 密钥认证和可信的 known_hosts；非 root 用户需要免密 sudo。脚本会打包当前工作区源码（包含本地修复），通过 SSH/SCP 上传，备份服务器源码，再执行安装或更新。无需先推送 GitHub。不会上传 `.env`、Git、私有小程序配置、构建产物、无关笔记或私有 `relay/proxy.py`。

已有 `/etc/protein-space/api.env`、`agent.env` 和项目 `.env` 会保留。首次安装自动生成随机 MySQL 密码和应用加密密钥，模型服务配置需自行填写；有占位符时退出码为 2，填好再执行同一命令。没有模型密钥时不会宣称真实模型已可用。

服务器配置路径：

```text
/opt/protein_space/.env          Docker MySQL 密码
/etc/protein-space/api.env       API 数据库、加密密钥及白名单
/etc/protein-space/agent.env     上游模型地址、API Key 和默认模型
```

新装需编辑 `agent.env` 中的 `AGENT_UPSTREAM_URL` 和 `AGENT_API_KEY`。如果已有自定义 MySQL 密码而缺少 api.env，还需填写 `MYSQL_DSN`。加密密钥必须保留，替换会使历史服务商密文无法解密。

## 服务器一键运行

```bash
cd /opt/protein_space
sudo bash scripts/install-host.sh
sudo bash scripts/update-host.sh
```

更新默认使用当前目录源码，适合上传包部署。Git 部署可使用 `sudo PULL=1 bash scripts/update-host.sh`，只允许干净工作区进行 fast-forward 拉取；`BRANCH` 默认为 main。

安装和更新会先完成 Go 测试、静态检查及编译；二进制通过原子替换激活，单个服务重启或健康检查失败时恢复其上一版二进制。安装过程不是数据库事务，多个组件失败时可能部分完成；查明原因后可重新运行。源码备份位于 backups，数据库持久卷和已有环境文件不会删除或重置。

## 各项目入口

| 目录 | 安装 | 更新 | 行为 |
| --- | --- | --- | --- |
| api | `sudo bash scripts/install-api.sh` | `sudo bash scripts/update-api.sh` | 测试、编译、systemd 与健康检查 |
| agent | `sudo bash scripts/install-agent.sh` | `sudo bash scripts/update-agent.sh` | 测试、编译、systemd 与健康检查 |
| frontend | `sudo bash scripts/install-frontend.sh` | `sudo bash scripts/update-frontend.sh` | 发布当前静态页面，应用 Nginx 配置 |
| nginx | `sudo bash scripts/install-nginx.sh` | `sudo bash scripts/update-nginx.sh` | 生成站点、校验、reload，失败恢复旧配置 |
| mysql | `sudo bash scripts/install-mysql.sh` | `sudo bash scripts/update-mysql.sh` | 启动现有 Compose 数据库并检查连接；不删除卷、不强制拉新镜像、不重建表 |
| relay | `sudo bash scripts/install-relay.sh` | `sudo bash scripts/update-relay.sh` | 仅内网中转机，须事先配置该机私有 proxy.py；不包含在全站安装中 |
| ios-app | `bash scripts/install-ios-app.sh` | `bash scripts/update-ios-app.sh` | macOS 上测试并构建模拟器 App；`SIMULATOR_ID` 可指定模拟器安装目标；真机需签名 |
| wechat-miniprogram | `bash scripts/install-wechat-miniprogram.sh` | `bash scripts/update-wechat-miniprogram.sh` | 运行测试、打开开发者工具；可指定 `WECHAT_CLI`，`CHECK_ONLY=1` 仅测试 |

除显式指定 `PULL=1` 的服务器更新外，各项目入口均使用当前源码，不自动拉代码。API 单独安装需先配置数据库和 Agent。iOS 脚本不安装 Xcode，微信脚本不代替 AppID、域名配置和发布审核。

## Nginx 接口

HTTP/HTTPS 站点统一引入 `/etc/nginx/snippets/protein-space-api.conf`，通过 `/api/` 前缀覆盖全部业务接口，保留原路径和查询参数：

```text
GET       /api/health
GET       /api/models
POST      /api/auth/register
POST      /api/auth/login
POST      /api/auth/logout
GET       /api/auth/me
GET/POST  /api/api-keys
GET/POST  /api/provider-configs
GET/POST  /api/admin/config
GET/POST/DELETE /api/auth/whitelist
GET/DELETE /api/conversations
GET       /api/conversations/messages
POST      /api/chat/stream
```

认证和权限检查由 API 执行。当前默认 `AUTH_REQUIRED=false` 允许访客读取模型和聊天；账号配置和管理接口仍需原有认证。Nginx 不改变权限。Agent 的 `/v1/` 为内部接口，公网返回 404。聊天关闭响应缓冲、缓存和压缩，读写超时 1 小时，上传限制 24MB。

检测到 `/etc/letsencrypt/live/$DOMAIN/` 证书时启用 HTTPS，HTTP 重定向 HTTPS；没有证书时只启用 HTTP，并不会自动申请证书。移动端要求有效 HTTPS，申请证书后执行 `sudo bash scripts/update-nginx.sh`。其他站点配置不会自动删除；若其他配置已声明相同域名，应先处理重复站点。

## 验证

```bash
sudo nginx -t
systemctl is-active protein-agent protein-api nginx
curl --fail http://127.0.0.1:8090/v1/health
curl --fail http://127.0.0.1:8080/api/health
curl --fail https://linpro.top/api/health
python3 -m unittest discover -s scripts/tests -p 'test_*.py'
```

健康检查不代替真实模型对话与持久化测试。线上对话验证需有效账号或个人 API Key。


## 连接 www.linpro.top，临时关闭聊天认证

客户端默认使用 `https://www.linpro.top`；原来保存的 `https://linpro.top` 自动迁移为 www 地址，自定义地址保留。Nginx 的默认 `DOMAIN=linpro.top` 同时覆盖这两个域名，不要为此把 DOMAIN 改成 www 域名，否则会改变默认的证书查找路径。

已有服务器执行：

```bash
cd /opt/protein_space
git pull --ff-only origin main
sudo bash scripts/update-host.sh
sudo bash scripts/set-chat-auth.sh off
curl --fail https://www.linpro.top/api/models
```

新安装默认关闭聊天认证。既有 api.env 的 AUTH_REQUIRED 值会保留，因此需显式执行 off；恢复时执行 `sudo bash scripts/set-chat-auth.sh on`。免登录模式向所有能访问站点的用户开放模型调用，访客只保留当前会话，不共享管理员身份或账号历史。有效登录/API Key 的历史功能保持不变。微信后台的 request 合法域名也需配置为 `https://www.linpro.top`。
