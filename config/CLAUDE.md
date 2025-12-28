# config/ 模块

> 🧭 [← 返回根目录](../CLAUDE.md) | 📦 kiro2api / config

## 模块职责

配置管理模块，包含模型映射、常量定义、调优参数、请求头构建。

## 文件清单

| 文件 | 职责 | 关键内容 |
|------|------|----------|
| `config.go` | 模型映射、URL 配置、请求头构建 | `ModelMap`, `GenerateMachineID()`, `BuildUserAgent()` |
| `constants.go` | 常量定义 | Token 管理、消息处理、EventStream 解析常量 |
| `tuning.go` | 调优参数 | 超时、缓存 TTL、解析器配置 |

## 版本配置

```go
var (
    KiroVersion   = getEnvWithDefault("KIRO_VERSION", "0.8.0")
    NodeVersion   = getEnvWithDefault("NODE_VERSION", "22.21.1")
    Region        = getEnvWithDefault("AWS_REGION", "us-east-1")
    SystemVersion = getSystemVersion() // 随机选择 darwin#24.6.0 或 win32#10.0.22631
)
```

## MachineID 生成

基于 refreshToken 的 SHA256 哈希生成，与 Kiro IDE 官方实现一致：

```go
// 生成规则: SHA256("KotlinNativeAPI/" + refreshToken)
func GenerateMachineID(refreshToken string) string

// 支持环境变量覆盖: MACHINE_ID (64字符)
```

## 请求头构建函数

| 函数 | 用途 | 格式示例 |
|------|------|----------|
| `BuildUserAgent(machineID)` | 主 User-Agent | `aws-sdk-js/1.0.27 ua/2.1 os/darwin#24.6.0 lang/js md/nodejs#22.21.1 api/codewhispererstreaming#1.0.27 m/E KiroIDE-0.8.0-{machineID}` |
| `BuildXAmzUserAgent(machineID)` | x-amz-user-agent | `aws-sdk-js/1.0.27 KiroIDE-0.8.0-{machineID}` |
| `BuildRefreshUserAgent(machineID)` | Token 刷新 | `KiroIDE-0.8.0-{machineID}` |
| `GenerateInvocationID()` | 请求 ID (UUID v4) | `xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx` |

## 动态域名函数

| 函数 | 返回值 |
|------|--------|
| `GetRefreshDomain()` | `prod.{region}.auth.desktop.kiro.dev` |
| `GetCodeWhispererDomain()` | `q.{region}.amazonaws.com` |
| `GetCodeWhispererURLV2()` | `https://q.{region}.amazonaws.com/generateAssistantResponse` |

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

| 环境变量 | 默认值 | 说明 |
|----------|--------|------|
| `KIRO_VERSION` | `0.8.0` | Kiro IDE 版本号 |
| `NODE_VERSION` | `22.21.1` | Node.js 版本号 |
| `AWS_REGION` | `us-east-1` | AWS 区域 |
| `SYSTEM_VERSION` | 随机 | 系统版本 (darwin#24.6.0 / win32#10.0.22631) |
| `MACHINE_ID` | 自动生成 | 64字符的 machineID |
| `MAX_TOOL_DESCRIPTION_LENGTH` | `10000` | 工具描述最大长度 |

## 外部服务 URL (静态常量)

| 常量 | URL |
|------|-----|
| `RefreshTokenURL` | `https://prod.us-east-1.auth.desktop.kiro.dev/refreshToken` |
| `IdcRefreshTokenURL` | `https://oidc.us-east-1.amazonaws.com/token` |
| `CodeWhispererURL` | `https://codewhisperer.us-east-1.amazonaws.com/generateAssistantResponse` |

> 注意：推荐使用动态函数 `GetCodeWhispererURLV2()` 和 `GetRefreshDomain()` 以支持多区域配置。

## 测试文件

- `model_test.go` - 模型映射测试

## 依赖关系

```
config/
├── ← auth/       (GenerateMachineID, BuildRefreshUserAgent, GetRefreshDomain)
├── ← converter/  (ModelMap, MaxToolDescriptionLength)
├── ← parser/     (EventStream 常量)
└── ← server/     (BuildUserAgent, BuildXAmzUserAgent, GetCodeWhispererURLV2, GenerateInvocationID)
```
