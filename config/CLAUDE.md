# config/ 模块

> 🧭 [← 返回根目录](../CLAUDE.md) | 📦 kiro2api / config

## 模块职责

配置管理模块，包含模型映射、常量定义、调优参数。

## 文件清单

| 文件 | 职责 | 关键内容 |
|------|------|----------|
| `config.go` | 模型映射和 URL 配置 | `ModelMap`, `RefreshTokenURL`, `CodeWhispererURL` |
| `constants.go` | 常量定义 | Token 管理、消息处理、EventStream 解析常量 |
| `tuning.go` | 调优参数 | 超时、缓存 TTL、解析器配置 |

## 模型映射

```go
var ModelMap = map[string]string{
    "claude-sonnet-4-5":          "CLAUDE_SONNET_4_5_20250929_V1_0",
    "claude-sonnet-4-5-20250929": "CLAUDE_SONNET_4_5_20250929_V1_0",
    "claude-sonnet-4-20250514":   "CLAUDE_SONNET_4_20250514_V1_0",
    "claude-3-7-sonnet-20250219": "CLAUDE_3_7_SONNET_20250219_V1_0",
    "claude-3-5-haiku-20241022":  "auto",
    "claude-haiku-4-5-20251001":  "auto",
}
```

## 关键常量

### Token 管理
| 常量 | 值 | 说明 |
|------|-----|------|
| `TokenCacheKeyFormat` | `"token_%d"` | 缓存 key 格式 |
| `TokenRefreshCleanupDelay` | `5s` | 刷新后清理延迟 |

### EventStream 解析
| 常量 | 值 | 说明 |
|------|-----|------|
| `EventStreamMinMessageSize` | `16` | 最小消息长度 |
| `EventStreamMaxMessageSize` | `16MB` | 最大消息长度 |

### Token 估算
| 常量 | 值 | 说明 |
|------|-----|------|
| `TokenEstimationRatio` | `4` | 字符到 token 比例 |
| `BaseToolsOverhead` | `100` | 工具基础开销 |

## 环境变量配置

```go
// 可通过环境变量覆盖的配置
var MaxToolDescriptionLength = getEnvIntWithDefault("MAX_TOOL_DESCRIPTION_LENGTH", 10000)
```

## 外部服务 URL

| 常量 | URL |
|------|-----|
| `RefreshTokenURL` | `https://prod.us-east-1.auth.desktop.kiro.dev/refreshToken` |
| `IdcRefreshTokenURL` | `https://oidc.us-east-1.amazonaws.com/token` |
| `CodeWhispererURL` | `https://codewhisperer.us-east-1.amazonaws.com/generateAssistantResponse` |

## 测试文件

- `model_test.go` - 模型映射测试

## 依赖关系

```
config/
├── ← auth/       (TokenCacheTTL, URL 常量)
├── ← converter/  (ModelMap, MaxToolDescriptionLength)
├── ← parser/     (EventStream 常量)
└── ← server/     (消息格式常量)
```
