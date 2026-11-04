# Protein Space 微信小程序

独立原生小程序，无 npm 依赖。支持 iPhone 微信中的文字对话、SSE 流式回复、模型选择、停止生成、新建对话，以及服务器历史对话的读取和续聊。

## 运行

1. 在微信开发者工具中导入本目录，使用你自己的小程序 AppID 替换 `project.config.json` 中的 `touristappid`。游客模式仅用于本地查看，不用于真机发布。
2. 在小程序管理后台的「开发管理 → 开发设置 → 服务器域名」中，将实际服务的 HTTPS 域名加入 request 合法域名，例如 `https://www.linpro.top`。使用有效 HTTPS 证书，服务端 `/api/` 需可从手机网络访问。
3. 编译，进入「设置」，默认站点为 `https://www.linpro.top`，免登录模式下 Key 留空，点击「连接服务」。地址不要加 `/api`。
4. 点击开发者工具的「预览」，用 iPhone 微信扫描二维码；正式发布需使用自己的小程序账号上传并完成微信审核。

个人 Key 只在页面内存中保留，重启小程序后重新输入；仅站点地址保存于本地。不要把模型服务商密钥放到客户端。

## 接口与行为

- `GET /api/models`：读取当前账户可用模型。
- `POST /api/chat/stream`：发送 `conversation_id`、`model`、完整文字消息上下文；处理 `delta`、`content`、`error` 和 `[DONE]`。
- `GET /api/conversations` 与 `GET /api/conversations/messages?id=…`：读取当前账户的服务器历史。
- 填写个人 Key 时请求携带 `X-API-Key`，留空时不发送该请求头；更换地址或 Key 后，点击连接服务会清空当前对话，避免混用账号上下文。
- 停止、超时或断线时保留已显示的部分回复，并提示回复可能不完整；部分回复是否落库取决于现有后端。历史记录依赖服务器 MySQL 和有效身份。
- 当前版本为文字聊天，不含微信授权登录、图片/文件上传、语音和 Markdown 富文本渲染。

## 本地验证

```bash
node --test wechat-miniprogram/utils/*.test.cjs
node --check wechat-miniprogram/pages/chat/chat.js
```

真机验收：连接服务 → 切换模型 → 发送中文和 emoji → 追问 → 停止长回复 → 新建对话 → 历史续聊；另检查无效 Key、无可用模型和断网提示。

网络接口参考：[wx.request](https://developers.weixin.qq.com/miniprogram/dev/api/network/request/wx.request.html)、[RequestTask.onChunkReceived](https://developers.weixin.qq.com/miniprogram/dev/api/network/request/RequestTask.onChunkReceived.html)。

## 一键检查/打开

从仓库根目录执行（不要在 `wechat-miniprogram/` 子目录执行）：

```bash
git pull --ff-only origin main
bash scripts/deploy.sh update wechat-miniprogram

# 只执行本地检查，不打开开发者工具
CHECK_ONLY=1 bash scripts/deploy.sh update wechat-miniprogram

# 自定义 CLI 路径；有空格时使用引号
WECHAT_CLI="/Applications/wechatwebdevtools.app/Contents/MacOS/cli" \
  bash scripts/deploy.sh update wechat-miniprogram
```

`install wechat-miniprogram` 与 `update wechat-miniprogram` 行为相同：运行工具函数及页面测试、语法检查，再打开开发者工具；不自动拉代码、更新后端、上传或发布。`CHECK_ONLY` 默认 0，设为 1 时仅测试；`WECHAT_CLI` 默认 `/Applications/wechatwebdevtools.app/Contents/MacOS/cli`。需要 Node.js 18+，脚本不支持 `--pull`。

AppID、基础库、域名、Key、模型、超时与可选线上联调参数的完整说明见 [主 README：微信小程序更新和参数](../README.md#5-微信小程序更新和参数)。个人 Key 创建参数见 [API 请求与认证参数](../README.md#6-api-请求与认证参数)。服务器部署共用后端，详见 [部署说明](../DEPLOYMENT.md)。

## 临时免登录模式

后端 `AUTH_REQUIRED=false`（未配置时也默认关闭聊天认证）允许直接读取模型和聊天，无需个人 Key。访客消息仅保留在当前会话，不读取或保存账号历史；使用有效个人 Key 时仍可访问自己的历史。管理后台和账号配置接口保留认证。已有服务器先更新代码，再执行 `sudo bash scripts/deploy.sh auth off`。恢复认证执行同一脚本的 `on` 参数。
