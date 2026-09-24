# 上游模型协议

模型与上游接口格式的映射集中在 [`model_protocols.go`](model_protocols.go) 的 `modelProtocolConfigs`。切换某个模型时，只需要修改该模型的 `Protocol`：

```go
{ID: "gpt-6-astra", Name: "GPT-6 Astra", Protocol: ProtocolResponses},
```

目前支持两种格式：

| 协议 | 请求地址 | 请求字段 | 流式输出格式 | 实现文件 |
| --- | --- | --- | --- | --- |
| `ProtocolResponses` | `/v1/responses` | `model`、`input`、`stream` | `response.output_text.delta` | `protocol_responses.go` |
| `ProtocolChatCompletions` | `/v1/chat/completions` | `model`、`messages`、`stream` | `choices[0].delta.content` | `protocol_chat_completions.go` |

两个协议都会被转换成 Agent 对前端统一的 `data: {"delta":"..."}` SSE 格式。Responses 接口不存在时会回退到 Chat Completions；鉴权、限流或服务端错误不会触发协议回退。

新增协议时：

1. 在 `model_protocols.go` 增加协议常量和模型映射。
2. 新建 `protocol_<name>.go`，实现请求转换和流式响应转换。
3. 在 `Service.proxyByModel` 中增加协议分支。
4. 在 `main_test.go` 增加模型映射、URL 和流式事件测试。

默认模型和上游地址仍由 `AGENT_MODEL`、`AGENT_UPSTREAM_URL` 配置；地址可以填写服务根地址、`/v1/chat/completions` 或 `/v1/responses`，Agent 会按协议自动转换路径。
