# Protein Space iPhone App

独立 SwiftUI 原生工程，最低 iOS 16，无第三方依赖。提供文字对话、SSE 流式回复、模型选择、停止生成、新建对话和服务器历史续聊。个人 API Key 存入本机 Keychain，站点地址存入 UserDefaults。

## 在 iPhone 上运行

1. 在 Mac 安装完整 Xcode，打开 `ProteinChat.xcodeproj`。
2. 选择 `ProteinChat` target，在「Signing & Capabilities」选择自己的开发 Team，将 Bundle Identifier `com.example.proteinspace.chat` 修改为你自己的唯一标识。
3. 连接 iPhone，按 Xcode 的提示完成设备配对及开发者模式设置，选择手机为运行目标并点击 Run。也可先选择 iPhone 模拟器运行。
4. App「连接设置」默认填入 `https://www.linpro.top`，改为实际服务器的 HTTPS 根地址（不要加 `/api`），当前免登录模式下 Key 留空，点击连接服务。
5. 选择模型并发送消息。历史记录与现有网页账户共享，依赖后端数据库。

工程未绑定开发 Team，不包含签名证书，也未生成已签名 IPA。正式分发需自行配置 App 图标、开发者签名和 App Store Connect / TestFlight 发布资料。

## 开发检查

在仓库根目录运行：

```bash
# 仅 Foundation 网络层及解析测试，Mac 命令行工具即可执行
swiftc ios-app/ProteinChat/ChatAPI.swift ios-app/tests/main.swift -o /tmp/protein-chat-tests
/tmp/protein-chat-tests

# 完整 UI 编译，需要已安装并选中完整 Xcode
xcodebuild -project ios-app/ProteinChat.xcodeproj -scheme ProteinChat \
  -sdk iphonesimulator -destination 'generic/platform=iOS Simulator' \
  -derivedDataPath /tmp/protein-chat-derived CODE_SIGNING_ALLOWED=NO build
```

建议真机验收：中文及 emoji 流式回复、多轮追问、切换模型、停止生成、历史续聊、无效 Key 和网络断开。当前环境只有 Command Line Tools，未进行 iOS SDK 编译及真机验收。

## 实现范围

通过 `X-API-Key` 调用现有 `/api/models`、`/api/chat/stream`、`/api/conversations` 和 `/api/conversations/messages`。使用 [URLSession](https://developer.apple.com/documentation/foundation/urlsession) 的异步字节流，按现有后端的单行 JSON `data:` 事件协议解析。更换连接会重置当前会话；停止或断线保留已显示内容并标记不完整，服务端是否保存部分回复由后端决定。

当前版本为文字聊天，不含账户注册、文件/图片上传、语音、Markdown 富文本及后台持续生成。不要在客户端填入模型服务商密钥。

## 一键构建/更新

在 macOS 执行 `bash scripts/install-ios-app.sh` 或 `bash scripts/update-ios-app.sh`，先运行网络层测试再构建模拟器 App；需要完整 Xcode。可通过 `SIMULATOR_ID` 指定模拟器安装目标。服务器只需部署共用后端，详见 [部署说明](../DEPLOYMENT.md)。

## 临时免登录模式

后端 `AUTH_REQUIRED=false`（未配置时也默认关闭聊天认证）允许直接读取模型和聊天，无需个人 Key。访客消息仅保留在当前会话，不读取或保存账号历史；使用有效个人 Key 时仍可访问自己的历史。管理后台和账号配置接口保留认证。已有服务器先更新代码，再执行 `sudo bash scripts/set-chat-auth.sh off`。恢复认证执行同一脚本的 `on` 参数。
