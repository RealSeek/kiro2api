# logger/ 模块

> 🧭 [← 返回根目录](../CLAUDE.md) | 📦 kiro2api / logger

## 模块职责

结构化 JSON 日志模块，支持多级别、多输出、调用栈追踪。

## 文件清单

| 文件 | 职责 |
|------|------|
| `logger.go` | 日志器实现 |

## 日志级别

```go
const (
    DEBUG Level = iota
    INFO
    WARN
    ERROR
    FATAL
)
```

## 使用方式

```go
// 基础日志
logger.Info("消息内容")
logger.Debug("调试信息", logger.String("key", "value"))
logger.Error("错误信息", logger.Err(err))

// 字段构造函数
logger.String(key, val)
logger.Int(key, val)
logger.Int64(key, val)
logger.Float64(key, val)
logger.Bool(key, val)
logger.Err(err)
logger.Duration(key, val)
logger.Any(key, val)
```

## 输出格式

```json
{
  "timestamp": "2025-12-28T18:00:00.000+08:00",
  "level": "INFO",
  "file": "server.go:123",
  "func": "StartServer",
  "message": "启动服务器",
  "port": "8080"
}
```

## 环境变量配置

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `LOG_LEVEL` | 日志级别 | `INFO` |
| `DEBUG` | 启用调试模式 | `false` |
| `LOG_FILE` | 日志文件路径 | (控制台) |
| `LOG_CONSOLE` | 是否输出到控制台 | `true` |
| `LOG_ENABLE_CALLER` | 启用调用栈信息 | `false` (DEBUG 时自动开启) |
| `LOG_CALLER_SKIP` | 调用栈深度 | `3` |

## 性能优化

- **原子操作**：日志级别使用 `atomic.Int64` 避免锁竞争
- **线程安全**：`log.Logger` 本身线程安全，无需额外 mutex
- **按需获取调用栈**：仅在 DEBUG 级别或显式开启时获取
- **字段排序**：动态字段按键名排序确保输出一致性

## 依赖关系

```
logger/
├── → bytedance/sonic  (JSON 序列化)
└── ← (所有模块)       (被全局使用)
```
