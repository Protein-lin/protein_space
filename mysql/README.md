# MySQL 配置说明

`init/001_schema.sql` 是 MySQL 8.0 的初始化脚本，包含：

- `users`：用户和登录密码哈希
- `provider_configs`：模型服务地址、加密后的 API Key、默认模型
- `user_provider_configs`：用户和 Key 配置的授权关系
- `conversations` / `messages`：持久化聊天记录
- `usage_records`：Token、耗时和调用状态统计

## 启动数据库

```bash
docker compose up -d mysql
```

默认连接信息仅用于本地开发：

```text
host: 127.0.0.1
port: 3306
database: waf_agent
user: waf
password: change-me-in-production
```

## API Key 安全要求

`provider_configs.api_key_ciphertext` 不保存明文。API 层应使用环境变量 `APP_ENCRYPTION_KEY`，通过 AES-GCM 等认证加密后写入；读取时在内存中解密并传给 Agent，响应中只返回 `api_key_hint`，例如 `...0ysxXBis`。

不要把 API Key 放入前端、Git、日志或 URL。之前已经发到聊天中的 Key 应立即撤销并重新生成。
