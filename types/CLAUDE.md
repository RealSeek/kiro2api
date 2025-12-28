# types/ 模块

> 🧭 [← 返回根目录](../CLAUDE.md) | 📦 kiro2api / types

## 模块职责

数据结构定义模块，包含所有 API 请求/响应的类型定义。

## 文件清单

| 文件 | 职责 | 关键类型 |
|------|------|----------|
| `anthropic.go` | Anthropic API 类型 | `AnthropicRequest`, `AnthropicTool`, `ContentBlock` |
| `openai.go` | OpenAI API 类型 | `OpenAIRequest`, `OpenAIMessage` |
| `codewhisperer.go` | CodeWhisperer 类型 | `CodeWhispererRequest`, `AssistantResponseEvent` |
| `codewhisperer_enums.go` | CW 枚举类型 | `ContentType`, `MessageStatus`, `UserIntent` |
| `token.go` | Token 相关类型 | `TokenInfo`, `TokenWithUsage` |
| `usage_limits.go` | 使用限制类型 | `UsageLimits`, `UsageBreakdown` |
| `count_tokens.go` | Token 计数类型 | `CountTokensRequest`, `CountTokensResponse` |
| `model.go` | 模型类型 | `Model`, `ModelsResponse` |
| `common.go` | 公共类型 | `Usage`, `ModelNotFoundErrorType` |

## 核心类型

### Anthropic 请求

```go
type AnthropicRequest struct {
    Model       string                    `json:"model"`
    MaxTokens   int                       `json:"max_tokens"`
    Messages    []AnthropicRequestMessage `json:"messages"`
    System      []AnthropicSystemMessage  `json:"system,omitempty"`
    Tools       []AnthropicTool           `json:"tools,omitempty"`
    ToolChoice  any                       `json:"tool_choice,omitempty"`
    Stream      bool                      `json:"stream"`
    Temperature *float64                  `json:"temperature,omitempty"`
}
```

### CodeWhisperer 请求

```go
type CodeWhispererRequest struct {
    ConversationState struct {
        AgentContinuationId string
        AgentTaskType       string
        ChatTriggerType     string
        CurrentMessage      struct {
            UserInputMessage struct {
                Content string
                ModelId string
                Images  []CodeWhispererImage
                UserInputMessageContext struct {
                    ToolResults []ToolResult
                    Tools       []CodeWhispererTool
                }
            }
        }
        ConversationId string
        History        []any
    }
}
```

### Token 信息

```go
type TokenInfo struct {
    AccessToken string
    ExpiresAt   time.Time
}

type TokenWithUsage struct {
    TokenInfo       TokenInfo
    UsageLimits     *UsageLimits
    AvailableCount  float64
    LastUsageCheck  time.Time
    IsUsageExceeded bool
}
```

### 使用限制

```go
type UsageLimits struct {
    UsageBreakdownList []UsageBreakdown
    UserInfo           UserInfo
}

type UsageBreakdown struct {
    ResourceType             string
    UsageLimitWithPrecision  float64
    CurrentUsageWithPrecision float64
    FreeTrialInfo            *FreeTrialInfo
}
```

## 枚举类型

### ContentType
- `ContentTypeMarkdown` = "markdown"
- `ContentTypePlain` = "plain"
- `ContentTypeJSON` = "json"

### MessageStatus
- `MessageStatusCompleted` = "completed"
- `MessageStatusInProgress` = "in_progress"
- `MessageStatusError` = "error"

### UserIntent
- `UserIntentExplainCodeSelection`
- `UserIntentSuggestAlternateImpl`
- `UserIntentApplyCommonBestPractices`
- `UserIntentImproveCode`
- `UserIntentShowExamples`
- `UserIntentCiteSources`
- `UserIntentExplainLineByLine`

## 依赖关系

```
types/
├── → bytedance/sonic  (JSON 序列化)
└── ← (所有模块)       (被全局使用)
```
