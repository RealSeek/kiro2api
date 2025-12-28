# kiro2api

将 Kiro IDE 的 AI 能力开放为标准 API，支持 Anthropic 和 OpenAI 格式。

## 功能

- Anthropic Messages API (`/v1/messages`)
- OpenAI Chat Completions API (`/v1/chat/completions`)
- 流式响应 (SSE)
- 工具调用 (Tool Use)
- 图片输入 (Base64)
- 多账号轮换
- Web 管理面板

## 支持的模型

| 模型 | ID |
|------|-----|
| Claude Opus 4.5 | `claude-opus-4-5-20251101` |
| Claude Sonnet 4.5 | `claude-sonnet-4-5-20250929` |
| Claude Sonnet 4 | `claude-sonnet-4-20250514` |
| Claude 3.7 Sonnet | `claude-3-7-sonnet-20250219` |
| Claude 3.5 Haiku | `claude-3-5-haiku-20241022` |

## 快速开始

### 1. 获取 Token

从 Kiro IDE 的 `~/.aws/sso/cache/` 目录获取 `refreshToken`。

### 2. 配置

```bash
cp .env.example .env
```

编辑 `.env`：

```bash
# 认证配置 (必需)
KIRO_AUTH_TOKEN='[{"auth":"Social","refreshToken":"你的token"}]'

# API 密钥 (客户端访问用)
KIRO_CLIENT_TOKEN=your-api-key
```

多账号配置：

```bash
KIRO_AUTH_TOKEN='[
  {"auth":"Social","refreshToken":"token1"},
  {"auth":"Social","refreshToken":"token2"},
  {"auth":"IdC","refreshToken":"token3","clientId":"xxx","clientSecret":"xxx"}
]'
```

或使用配置文件：

```bash
KIRO_AUTH_TOKEN=./auth_config.json
```

### 3. 运行

```bash
# 编译
go build -o kiro2api

# 运行
./kiro2api
```

Docker：

```bash
docker-compose up -d
```

## 使用

### Claude Code

```bash
export ANTHROPIC_BASE_URL=http://localhost:8080
export ANTHROPIC_API_KEY=your-api-key
```

### API 调用

```bash
curl http://localhost:8080/v1/messages \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-4-20250514",
    "max_tokens": 1024,
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```

## 配置项

| 环境变量 | 说明 | 默认值 |
|----------|------|--------|
| `KIRO_AUTH_TOKEN` | 认证配置 (JSON 或文件路径) | - |
| `KIRO_CLIENT_TOKEN` | API 访问密钥 | - |
| `PORT` | 服务端口 | 8080 |
| `LOG_LEVEL` | 日志级别 | info |
| `ADMIN_PASSWORD` | 管理面板密码 | - |

## API 端点

| 路径 | 说明 |
|------|------|
| `GET /` | 管理面板 |
| `GET /v1/models` | 模型列表 |
| `POST /v1/messages` | Anthropic API |
| `POST /v1/chat/completions` | OpenAI API |
| `GET /api/tokens` | Token 状态 |

## License

MIT
