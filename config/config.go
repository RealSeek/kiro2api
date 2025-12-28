package config

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	mathrand "math/rand"
	"os"
	"strconv"
	"sync"
)

// 版本配置常量（参考 kiro.rs）
var (
	// KiroVersion Kiro IDE 版本号
	KiroVersion = getEnvWithDefault("KIRO_VERSION", "0.8.0")

	// NodeVersion Node.js 版本号
	NodeVersion = getEnvWithDefault("NODE_VERSION", "22.21.1")

	// Region AWS 区域
	Region = getEnvWithDefault("AWS_REGION", "us-east-1")

	// SystemVersion 系统版本（随机选择）
	SystemVersion = getSystemVersion()
)

// 系统版本列表
var systemVersions = []string{"darwin#24.6.0", "win32#10.0.22631"}

// getSystemVersion 获取系统版本（支持环境变量覆盖或随机选择）
func getSystemVersion() string {
	if v := os.Getenv("SYSTEM_VERSION"); v != "" {
		return v
	}
	return systemVersions[mathrand.Intn(len(systemVersions))]
}

// MachineID 生成器（基于 refreshToken 的 SHA256）
var (
	machineIDCache     = make(map[string]string)
	machineIDCacheLock sync.RWMutex
)

// GenerateMachineID 根据 refreshToken 生成 machine_id
// 优先使用环境变量 MACHINE_ID，否则基于 refreshToken 生成 SHA256
func GenerateMachineID(refreshToken string) string {
	// 优先使用环境变量配置的 machineId
	if machineID := os.Getenv("MACHINE_ID"); machineID != "" && len(machineID) == 64 {
		return machineID
	}

	if refreshToken == "" {
		return ""
	}

	// 检查缓存
	machineIDCacheLock.RLock()
	if cached, ok := machineIDCache[refreshToken]; ok {
		machineIDCacheLock.RUnlock()
		return cached
	}
	machineIDCacheLock.RUnlock()

	// 生成 SHA256（与 kiro.rs 保持一致）
	input := "KotlinNativeAPI/" + refreshToken
	hash := sha256.Sum256([]byte(input))
	machineID := hex.EncodeToString(hash[:])

	// 缓存结果
	machineIDCacheLock.Lock()
	machineIDCache[refreshToken] = machineID
	machineIDCacheLock.Unlock()

	return machineID
}

// BuildUserAgent 构建 User-Agent 请求头
func BuildUserAgent(machineID string) string {
	return "aws-sdk-js/1.0.27 ua/2.1 os/" + SystemVersion + " lang/js md/nodejs#" + NodeVersion + " api/codewhispererstreaming#1.0.27 m/E KiroIDE-" + KiroVersion + "-" + machineID
}

// BuildXAmzUserAgent 构建 x-amz-user-agent 请求头
func BuildXAmzUserAgent(machineID string) string {
	return "aws-sdk-js/1.0.27 KiroIDE-" + KiroVersion + "-" + machineID
}

// BuildRefreshUserAgent 构建 Token 刷新请求的 User-Agent
func BuildRefreshUserAgent(machineID string) string {
	return "KiroIDE-" + KiroVersion + "-" + machineID
}

// getEnvWithDefault 获取字符串类型环境变量（带默认值）
func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// GetRefreshDomain 获取 Token 刷新域名
func GetRefreshDomain() string {
	return "prod." + Region + ".auth.desktop.kiro.dev"
}

// GetCodeWhispererDomain 获取 CodeWhisperer API 域名
func GetCodeWhispererDomain() string {
	// 使用 q.{region}.amazonaws.com 格式（参考 kiro.rs）
	return "q." + Region + ".amazonaws.com"
}

// GetCodeWhispererURLV2 获取 CodeWhisperer API URL（新版）
func GetCodeWhispererURLV2() string {
	return "https://" + GetCodeWhispererDomain() + "/generateAssistantResponse"
}

// GenerateInvocationID 生成请求ID (UUID v4格式，按照AWS文档规范)
func GenerateInvocationID() string {
	var b [16]byte
	rand.Read(b[:])
	// 设置 UUID v4 格式位
	b[6] = (b[6] & 0x0F) | 0x40 // Version 4
	b[8] = (b[8] & 0x3F) | 0x80 // Variant bits

	var buf [32]byte
	hex.Encode(buf[:], b[:])
	s := string(buf[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", s[0:8], s[8:12], s[12:16], s[16:20], s[20:32])
}

// ModelMap 模型映射表
var ModelMap = map[string]string{
	"claude-sonnet-4-5":          "CLAUDE_SONNET_4_5_20250929_V1_0",
	"claude-sonnet-4-5-20250929": "CLAUDE_SONNET_4_5_20250929_V1_0",
	"claude-sonnet-4-20250514":   "CLAUDE_SONNET_4_20250514_V1_0",
	"claude-3-7-sonnet-20250219": "CLAUDE_3_7_SONNET_20250219_V1_0",
	"claude-3-5-haiku-20241022":  "auto",
	"claude-haiku-4-5-20251001":  "auto",
	"claude-opus-4-5-20251101":   "CLAUDE_OPUS_4_5_20251101_V1_0",
}

// RefreshTokenURL 刷新token的URL (social方式)
const RefreshTokenURL = "https://prod.us-east-1.auth.desktop.kiro.dev/refreshToken"

// IdcRefreshTokenURL IdC认证方式的刷新token URL
const IdcRefreshTokenURL = "https://oidc.us-east-1.amazonaws.com/token"

// CodeWhispererURL CodeWhisperer API的URL
const CodeWhispererURL = "https://codewhisperer.us-east-1.amazonaws.com/generateAssistantResponse"

// MaxToolDescriptionLength 工具描述的最大长度（字符数）
// 可通过环境变量 MAX_TOOL_DESCRIPTION_LENGTH 配置，默认 10000
var MaxToolDescriptionLength = getEnvIntWithDefault("MAX_TOOL_DESCRIPTION_LENGTH", 10000)

// getEnvIntWithDefault 获取整数类型环境变量（带默认值）
func getEnvIntWithDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
