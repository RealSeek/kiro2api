package auth

import (
	"fmt"
	"io"
	"kiro2api/config"
	"kiro2api/logger"
	"kiro2api/types"
	"kiro2api/utils"
	"net/http"
	"net/url"
)

// UsageLimitsChecker 使用限制检查器 (遵循SRP原则)
type UsageLimitsChecker struct {
	httpClient *http.Client
}

// NewUsageLimitsChecker 创建使用限制检查器
func NewUsageLimitsChecker() *UsageLimitsChecker {
	return &UsageLimitsChecker{
		httpClient: utils.SharedHTTPClient,
	}
}

// CheckUsageLimits 检查token的使用限制 (基于token.md API规范)
func (c *UsageLimitsChecker) CheckUsageLimits(token types.TokenInfo) (*types.UsageLimits, error) {
	// 构建请求URL (使用新的域名格式)
	baseURL := "https://" + config.GetCodeWhispererDomain() + "/getUsageLimits"
	params := url.Values{}
	params.Add("isEmailRequired", "true")
	params.Add("origin", "AI_EDITOR")
	params.Add("resourceType", "AGENTIC_REQUEST")

	requestURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	// 创建HTTP请求
	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建使用限制检查请求失败: %v", err)
	}

	// 生成 machineID（基于 refreshToken）
	machineID := config.GenerateMachineID(token.RefreshToken)

	// 设置请求头 (参考 kiro.rs)
	req.Header.Set("x-amz-user-agent", config.BuildXAmzUserAgent(machineID))
	req.Header.Set("User-Agent", config.BuildUserAgent(machineID))
	req.Header.Set("Host", config.GetCodeWhispererDomain())
	req.Header.Set("amz-sdk-invocation-id", config.GenerateInvocationID())
	req.Header.Set("amz-sdk-request", "attempt=1; max=1")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token.AccessToken))
	req.Header.Set("Connection", "close")

	// 发送请求
	logger.Debug("发送使用限制检查请求",
		logger.String("url", requestURL),
		logger.String("token_preview", token.AccessToken[:20]+"..."))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("使用限制检查请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取使用限制响应失败: %v", err)
	}

	logger.Debug("使用限制API响应",
		logger.Int("status_code", resp.StatusCode),
		logger.String("response_body", string(body)))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("使用限制检查失败: 状态码 %d, 响应: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var usageLimits types.UsageLimits
	if err := utils.SafeUnmarshal(body, &usageLimits); err != nil {
		return nil, fmt.Errorf("解析使用限制响应失败: %v", err)
	}

	// 记录关键信息
	c.logUsageLimits(&usageLimits)

	return &usageLimits, nil
}

// logUsageLimits 记录使用限制的关键信息
func (c *UsageLimitsChecker) logUsageLimits(limits *types.UsageLimits) {
	for _, breakdown := range limits.UsageBreakdownList {
		if breakdown.ResourceType == "CREDIT" {
			// 计算可用次数 (使用浮点精度数据)
			var totalLimit float64
			var totalUsed float64

			// 基础额度
			baseLimit := breakdown.UsageLimitWithPrecision
			baseUsed := breakdown.CurrentUsageWithPrecision
			totalLimit += baseLimit
			totalUsed += baseUsed

			// 免费试用额度
			var freeTrialLimit float64
			var freeTrialUsed float64
			if breakdown.FreeTrialInfo != nil && breakdown.FreeTrialInfo.FreeTrialStatus == "ACTIVE" {
				freeTrialLimit = breakdown.FreeTrialInfo.UsageLimitWithPrecision
				freeTrialUsed = breakdown.FreeTrialInfo.CurrentUsageWithPrecision
				totalLimit += freeTrialLimit
				totalUsed += freeTrialUsed
			}

			available := totalLimit - totalUsed

			logger.Info("CREDIT使用状态",
				logger.String("resource_type", breakdown.ResourceType),
				logger.Float64("total_limit", totalLimit),
				logger.Float64("total_used", totalUsed),
				logger.Float64("available", available),
				logger.Float64("base_limit", baseLimit),
				logger.Float64("base_used", baseUsed),
				logger.Float64("free_trial_limit", freeTrialLimit),
				logger.Float64("free_trial_used", freeTrialUsed),
				logger.String("free_trial_status", func() string {
					if breakdown.FreeTrialInfo != nil {
						return breakdown.FreeTrialInfo.FreeTrialStatus
					}
					return "NONE"
				}()))

			if available <= 1 {
				logger.Warn("CREDIT使用量即将耗尽",
					logger.Float64("remaining", available),
					logger.String("recommendation", "考虑切换到其他token"))
			}

			break
		}
	}

	// 记录订阅信息
	logger.Debug("订阅信息",
		logger.String("subscription_type", limits.SubscriptionInfo.Type),
		logger.String("subscription_title", limits.SubscriptionInfo.SubscriptionTitle),
		logger.String("user_email", limits.UserInfo.Email))
}
