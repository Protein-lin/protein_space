# Python 内网中转代理

这是一个零第三方依赖的 HTTP/SSE 反向代理，适合部署在内网中转服务器上。它轮询 `UPSTREAMS`，保留请求体和 `Authorization`，并以流式方式转发响应。

启动：

```bash
cd /opt/protein_space
RELAY_BIND=127.0.0.1 \
RELAY_PORT=8443 \
UPSTREAMS=https://10.0.0.21,https://10.0.0.22 \
UPSTREAM_HOST=ai-api.ort.sealaly.com \
python3 relay/proxy.py
```

然后让内网 `frpc` 转发：

```toml
[[proxies]]
name = "internal-model-relay"
type = "tcp"
localIP = "127.0.0.1"
localPort = 8443
remotePort = 16443
```

这个示例代理监听的是普通 HTTP（TLS 在内网模型服务和 Python 代理之间由 urllib 发起），所以公网 Agent 使用：

```env
AGENT_UPSTREAM_URL=http://ai-api.ort.sealaly.com:16443/v1/chat/completions
```

公网服务器 `/etc/hosts`：

```text
127.0.0.1 ai-api.ort.sealaly.com
```

如果上游是自签名证书，请把内网 CA 加入系统信任库；不要在生产环境关闭 TLS 校验。代理默认只监听 `127.0.0.1`，不要直接绑定公网地址。
