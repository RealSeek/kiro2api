# utils/ 模块

> 🧭 [← 返回根目录](../CLAUDE.md) | 📦 kiro2api / utils

## 模块职责

通用工具模块，提供 HTTP 工具、Token 估算、消息处理、会话 ID 管理等功能。

## 文件清单

| 文件 | 职责 | 关键函数/类型 |
|------|------|---------------|
| `token_estimator.go` | Token 数量估算 | `TokenEstimator`, `EstimateTokens()`, `EstimateTextTokens()` |
| `request_analyzer.go` | 请求复杂度分析 | `RequestComplexity`, `AnalyzeRequestComplexity()` |
| `conversation_id.go` | 会话 ID 管理 | `ConversationIDManager`, `GenerateStableConversationID()` |
| `message.go` | 消息内容处理 | `GetMessageContent()`, `ParseToolResultContent()` |
| `http.go` | HTTP 响应读取 | `ReadHTTPResponse()` |
| `json.go` | JSON 序列化 | `SafeMarshal()` |
| `uuid.go` | UUID 生成 | `GenerateUUID()` |
| `image.go` | 图片处理 | 图片格式转换 |
| `env.go` | 环境变量 | 环境变量读取工具 |
| `common.go` | 通用工具 | `IntMin()`, `IntMax()` |
| `client.go` | HTTP 客户端 | HTTP 客户端工具 |
| `token_refresh_manager.go` | Token 刷新管理 | Token 刷新调度 |

## 核心功能

### Token 估算器

```go
type TokenEstimator struct{}

// 估算请求的 token 数量
func (e *TokenEstimator) EstimateTokens(req *types.CountTokensRequest) int

// 估算纯文本的 token 数量
func (e *TokenEstimator) EstimateTextTokens(text string) int

// 估算工具调用的 token 数量
func (e *TokenEstimator) EstimateToolUseTokens(toolName string, toolInput map[string]any) int
```

**估算算法**：
- 英文：约 4 字符/token
- 中文：约 1.5 字符/token（纯中文有基础开销）
- 工具：名称 + 描述 + Schema（自适应密度）
- 长文本压缩：50-1000+ 字符分段压缩

### 请求复杂度分析

```go
type RequestComplexity int

const (
    SimpleRequest  RequestComplexity = iota  // 简单请求
    ComplexRequest                           // 复杂请求
)

func AnalyzeRequestComplexity(req types.AnthropicRequest) RequestComplexity
```

**复杂度评分因素**：
- MaxTokens > 4000：+2 分
- 内容长度 > 10K：+2 分
- 使用工具：+2 分
- 系统提示 > 2K：+1 分
- 包含复杂任务关键词：+1 分
- 总分 ≥ 3：复杂请求

### 会话 ID 管理

```go
type ConversationIDManager struct {
    mu    sync.RWMutex
    cache map[string]string
}

// 生成稳定的会话 ID（基于客户端特征 + 时间窗口）
func GenerateStableConversationID(ctx *gin.Context) string

// 生成稳定的代理延续 ID（GUID 格式）
func GenerateStableAgentContinuationID(ctx *gin.Context) string
```

**ID 生成策略**：
- 优先使用自定义头：`X-Conversation-ID`、`X-Agent-Continuation-ID`
- 基于客户端特征：IP + UserAgent + 时间窗口（小时级）
- MD5 哈希生成确定性 ID

### 消息内容处理

```go
// 从消息中提取文本内容（支持多种格式）
func GetMessageContent(content any) (string, error)

// 解析 tool_result 的 content 字段
func ParseToolResultContent(content any) string
```

**支持的内容类型**：
- `string`：纯文本
- `[]any`：内容块数组
- `[]types.ContentBlock`：类型化内容块
- `map[string]any`：结构化对象

## 测试文件

- `token_estimator_test.go` - Token 估算测试
- `request_analyzer_test.go` - 请求分析测试
- `conversation_id_test.go` - 会话 ID 测试
- `conversation_id_race_test.go` - 并发安全测试
- `http_test.go` - HTTP 工具测试
- `image_test.go` - 图片处理测试
- `env_test.go` - 环境变量测试
- `uuid_test.go` - UUID 生成测试
- `safe_marshal_test.go` - JSON 序列化测试
- `token_refresh_manager_test.go` - Token 刷新测试

## 依赖关系

```
utils/
├── → config/       (TokenEstimationRatio, LongTextThreshold)
├── → types/        (AnthropicRequest, ContentBlock)
├── → bytedance/sonic (JSON 序列化)
├── → gin-gonic/gin (HTTP 上下文)
├── ← converter/    (GetMessageContent, GenerateStableConversationID)
├── ← server/       (RequestComplexity, TokenEstimator)
└── ← auth/         (TokenRefreshManager)
```
