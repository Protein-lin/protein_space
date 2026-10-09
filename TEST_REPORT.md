# 功能回归记录（2026-10-09）

结论：本轮可执行的自动化测试通过，发现并修复四类问题；尚不能认定所有线上功能或移动端真机功能均正常。修复仅位于本地工作区，未部署。

## 本轮修复

1. API 日志中间件包装 ResponseWriter 后丢失 http.Flusher，导致 SSE 无法及时刷新。新增 Flush 转发并验证实际刷新。
2. Chat Completions 和 Responses 上游未发结束事件便关闭连接时，被当成成功。现在返回 unexpected EOF，交由现有错误事件处理提示失败。
3. Chat Completions 单行 SSE 超过默认 64KB 会失败。缓冲上限与 Responses 对齐为 1MB，90KB 中文事件回归通过。
4. POST /api/provider-configs 的四个字段误共用 default_model JSON 标签，正常请求返回 400。修正字段映射，并验证模拟数据库事务、用户关联和密钥加密。

## 已验证

| 范围 | 结果 | 边界 |
| --- | --- | --- |
| 网页、小程序自动化 | 32 项全部通过 | 包含聊天、模型选择、停止、历史续聊、删除、附件构造与大小/数量限制、HTML 转义、UTF-8 分片、异常恢复；使用模拟 DOM/微信接口 |
| Go API | race 测试通过，语句覆盖率 27.7%；go vet 通过 | 增加鉴权拒绝、白名单代理来源、模型校验、模拟 Agent 流式转发、附件存储编码、服务商配置测试；数据库使用测试替身 |
| Go Agent | race 测试通过，语句覆盖率 40.4%；go vet 通过 | 协议映射、两种协议的流式成功/断流/错误/长事件、图片转换；上游为本地模拟服务 |
| iOS Foundation 网络层 | Swift 编译及 API/SSE 检查通过 | 不等于 SwiftUI 工程或真机验收 |
| 语法 | JS 和两个部署脚本检查通过 | 未执行服务器安装或更新 |
| 线上健康接口 | HTTP 200，status=ok，database=true | 仅健康接口报告数据库已配置/连接状态，不验证完整数据库业务 |
| 线上网页 | 首页、聊天入口、登录弹窗正常打开 | 未登录状态模型选择禁用 |

## 未完成的实测

- 线上冒烟脚本在读取模型时收到 HTTP 401，未创建测试会话。需要有效测试账号/API Key 后验证真实模型、连续对话、MySQL 历史落库和删除。
- 注册、登录成功、退出、API Key 创建、服务商配置真实落库、管理后台的用户/权限/白名单变更，尚未用测试账号完整验收。
- 图片/文件的真实模型理解、所有可用模型逐一调用、网络中断与重连实测，尚未完成。
- 本机无完整 Xcode，仅有 Command Line Tools；未执行 iOS SDK 编译、模拟器及 iPhone 真机验收。微信开发者工具及微信真机也未验收。
- Nginx、MySQL 初始化/迁移、部署脚本及可选 relay 未做运行测试。本机未找到 Docker。

## 复现本地检查

```sh
node --test frontend/app.test.cjs wechat-miniprogram/utils/*.test.cjs wechat-miniprogram/tests/chat.test.cjs
(cd api && go test -race -cover ./... && go vet ./...)
(cd agent && go test -race -cover ./... && go vet ./...)
swiftc -module-cache-path /private/tmp/protein-swift-check-oct09 ios-app/ProteinChat/ChatAPI.swift ios-app/tests/main.swift -o /private/tmp/protein-chat-tests-oct09
/private/tmp/protein-chat-tests-oct09
bash -n scripts/install-host.sh scripts/update-host.sh
```

本机 PATH 中 go 为 1.19.4，但 GOROOT 指向 1.22.0。本轮明确使用 `/Users/linyangzhi/.asdf/installs/golang/1.22.0/go/bin/go`、`GOCACHE=/private/tmp/protein-go-cache`，并为 test 添加 `-ldflags=-linkmode=external`，避开旧 Go 内部链接器在当前 macOS 下的 LC_UUID 错误。链接器仍有 LC_DYSYMTAB 警告，但测试及竞态检查通过。测试模拟服务需要临时回环端口权限。Swift 使用新的规范化缓存路径，避开旧缓存中 /tmp 与 /private/tmp 路径冲突。
