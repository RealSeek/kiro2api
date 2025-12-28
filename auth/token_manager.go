package auth

import (
	"fmt"
	"kiro2api/config"
	"kiro2api/logger"
	"kiro2api/types"
	"sync"
	"time"
)

// TokenManager 简化的token管理器
type TokenManager struct {
	cache        *SimpleTokenCache
	configs      []AuthConfig
	mutex        sync.RWMutex
	configOrder  []string        // 配置顺序
	currentIndex int             // 当前使用的token索引
	exhausted    map[string]bool // 已耗尽的token记录
	refreshing   map[string]bool // 正在刷新的token记录
}

// SimpleTokenCache 简化的token缓存（纯数据结构，无锁）
// 所有并发访问由 TokenManager.mutex 统一管理
type SimpleTokenCache struct {
	tokens map[string]*CachedToken
	ttl    time.Duration
}

// CachedToken 缓存的token信息
type CachedToken struct {
	Token     types.TokenInfo
	UsageInfo *types.UsageLimits
	CachedAt  time.Time
	LastUsed  time.Time
	Available float64
}

// NewSimpleTokenCache 创建简单的token缓存
func NewSimpleTokenCache(ttl time.Duration) *SimpleTokenCache {
	return &SimpleTokenCache{
		tokens: make(map[string]*CachedToken),
		ttl:    ttl,
	}
}

// NewTokenManager 创建新的token管理器
func NewTokenManager(configs []AuthConfig) *TokenManager {
	// 生成配置顺序
	configOrder := generateConfigOrder(configs)

	logger.Info("TokenManager初始化（按需刷新策略）",
		logger.Int("config_count", len(configs)),
		logger.Int("config_order_count", len(configOrder)))

	return &TokenManager{
		cache:        NewSimpleTokenCache(config.TokenCacheTTL),
		configs:      configs,
		configOrder:  configOrder,
		currentIndex: 0,
		exhausted:    make(map[string]bool),
		refreshing:   make(map[string]bool),
	}
}

// getBestToken 获取最优可用token
// 按需刷新：只刷新当前选中的token，不刷新全部
func (tm *TokenManager) getBestToken() (types.TokenInfo, error) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	// 选择最优token（内部方法，不加锁）
	bestToken := tm.selectBestTokenUnlocked()
	if bestToken == nil {
		return types.TokenInfo{}, fmt.Errorf("没有可用的token")
	}

	// 更新最后使用时间（在锁内，安全）
	bestToken.LastUsed = time.Now()
	if bestToken.Available > 0 {
		bestToken.Available--
	}

	return bestToken.Token, nil
}

// GetBestTokenWithUsage 获取最优可用token（包含使用信息）
// 按需刷新：只刷新当前选中的token，不刷新全部
func (tm *TokenManager) GetBestTokenWithUsage() (*types.TokenWithUsage, error) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	// 选择最优token（内部方法，不加锁）
	bestToken := tm.selectBestTokenUnlocked()
	if bestToken == nil {
		return nil, fmt.Errorf("没有可用的token")
	}

	// 更新最后使用时间（在锁内，安全）
	bestToken.LastUsed = time.Now()
	available := bestToken.Available
	if bestToken.Available > 0 {
		bestToken.Available--
	}

	// 构造 TokenWithUsage
	tokenWithUsage := &types.TokenWithUsage{
		TokenInfo:       bestToken.Token,
		UsageLimits:     bestToken.UsageInfo,
		AvailableCount:  available, // 使用精确计算的可用次数
		LastUsageCheck:  bestToken.LastUsed,
		IsUsageExceeded: available <= 0,
	}

	logger.Debug("返回TokenWithUsage",
		logger.Float64("available_count", available),
		logger.Bool("is_exceeded", tokenWithUsage.IsUsageExceeded))

	return tokenWithUsage, nil
}

// selectBestTokenUnlocked 按配置顺序选择下一个可用token
// 内部方法：调用者必须持有 tm.mutex
// 按需刷新：当选中的token缓存过期时，触发该token的异步刷新
func (tm *TokenManager) selectBestTokenUnlocked() *CachedToken {
	// 调用者已持有 tm.mutex，无需额外加锁

	// 如果没有配置顺序，返回nil
	if len(tm.configOrder) == 0 {
		return nil
	}

	// 从当前索引开始，找到第一个可用的token
	for attempts := 0; attempts < len(tm.configOrder); attempts++ {
		currentKey := tm.configOrder[tm.currentIndex]
		currentIdx := tm.currentIndex

		// 检查这个token是否存在于缓存中
		cached, exists := tm.cache.tokens[currentKey]

		if exists {
			// 检查缓存是否过期
			cacheExpired := time.Since(cached.CachedAt) > tm.cache.ttl

			if cacheExpired {
				// 缓存过期，触发异步刷新（如果没有正在刷新）
				if !tm.refreshing[currentKey] {
					tm.triggerAsyncRefreshUnlocked(currentIdx, currentKey)
				}
				// 即使缓存过期，如果token本身还可用，仍然返回它
				if cached.IsUsable() {
					logger.Debug("使用过期缓存的token（已触发异步刷新）",
						logger.String("cache_key", currentKey),
						logger.Int("index", currentIdx))
					return cached
				}
			} else {
				// 缓存未过期，检查token是否可用
				if cached.IsUsable() {
					logger.Debug("顺序策略选择token",
						logger.String("selected_key", currentKey),
						logger.Int("index", currentIdx),
						logger.Float64("available_count", cached.Available))
					return cached
				}
			}
		} else {
			// 缓存中不存在，触发异步刷新
			if !tm.refreshing[currentKey] {
				tm.triggerAsyncRefreshUnlocked(currentIdx, currentKey)
			}
		}

		// 标记当前token为已耗尽，移动到下一个
		tm.exhausted[currentKey] = true
		tm.currentIndex = (tm.currentIndex + 1) % len(tm.configOrder)

		logger.Debug("token不可用，切换到下一个",
			logger.String("exhausted_key", currentKey),
			logger.Int("next_index", tm.currentIndex))
	}

	// 所有token都不可用
	logger.Warn("所有token都不可用",
		logger.Int("total_count", len(tm.configOrder)),
		logger.Int("exhausted_count", len(tm.exhausted)))

	return nil
}

// triggerAsyncRefreshUnlocked 触发单个token的异步刷新
// 内部方法：调用者必须持有 tm.mutex
func (tm *TokenManager) triggerAsyncRefreshUnlocked(index int, cacheKey string) {
	if index < 0 || index >= len(tm.configs) {
		return
	}

	cfg := tm.configs[index]
	if cfg.Disabled {
		return
	}

	// 标记为正在刷新
	tm.refreshing[cacheKey] = true

	// 异步刷新
	go tm.refreshSingleTokenAsync(index, cfg)

	logger.Debug("触发单个token异步刷新",
		logger.String("cache_key", cacheKey),
		logger.Int("index", index))
}

// refreshSingleTokenAsync 异步刷新单个token并更新缓存
func (tm *TokenManager) refreshSingleTokenAsync(index int, cfg AuthConfig) {
	cacheKey := fmt.Sprintf(config.TokenCacheKeyFormat, index)

	// 确保完成后清除刷新标记
	defer func() {
		tm.mutex.Lock()
		delete(tm.refreshing, cacheKey)
		tm.mutex.Unlock()
	}()

	// 刷新token
	token, err := tm.refreshSingleToken(cfg)
	if err != nil {
		logger.Warn("刷新单个token失败",
			logger.Int("config_index", index),
			logger.String("auth_type", cfg.AuthType),
			logger.Err(err))
		return
	}

	// 检查使用限制
	var usageInfo *types.UsageLimits
	var available float64

	checker := NewUsageLimitsChecker()
	if usage, checkErr := checker.CheckUsageLimits(token); checkErr == nil {
		usageInfo = usage
		available = CalculateAvailableCount(usage)
	} else {
		logger.Warn("检查使用限制失败", logger.Err(checkErr))
	}

	// 更新缓存（需要加锁）
	tm.mutex.Lock()
	tm.cache.tokens[cacheKey] = &CachedToken{
		Token:     token,
		UsageInfo: usageInfo,
		CachedAt:  time.Now(),
		Available: available,
	}
	// 清除该token的耗尽标记
	delete(tm.exhausted, cacheKey)
	tm.mutex.Unlock()

	logger.Debug("token缓存更新",
		logger.String("cache_key", cacheKey),
		logger.Float64("available", available))
}

// IsUsable 检查缓存的token是否可用
func (ct *CachedToken) IsUsable() bool {
	// 检查token是否过期
	if time.Now().After(ct.Token.ExpiresAt) {
		return false
	}

	// 检查可用次数
	return ct.Available > 0
}

// TokenCacheStatus 缓存状态信息（用于 Dashboard 显示）
type TokenCacheStatus struct {
	Index     int
	Cached    bool                // 是否有缓存
	Token     types.TokenInfo     // Token 信息
	UsageInfo *types.UsageLimits  // 使用限制信息
	Available float64             // 可用次数
	CachedAt  time.Time           // 缓存时间
	LastUsed  time.Time           // 最后使用时间
	Error     string              // 错误信息（如果有）
}

// GetAllCacheStatus 获取所有 Token 的缓存状态（只读，不触发刷新）
func (tm *TokenManager) GetAllCacheStatus() []TokenCacheStatus {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()

	result := make([]TokenCacheStatus, len(tm.configs))

	for i := range tm.configs {
		cacheKey := fmt.Sprintf(config.TokenCacheKeyFormat, i)
		status := TokenCacheStatus{
			Index:  i,
			Cached: false,
		}

		if cached, exists := tm.cache.tokens[cacheKey]; exists {
			status.Cached = true
			status.Token = cached.Token
			status.UsageInfo = cached.UsageInfo
			status.Available = cached.Available
			status.CachedAt = cached.CachedAt
			status.LastUsed = cached.LastUsed
		}

		result[i] = status
	}

	return result
}

// *** 已删除 set 和 updateLastUsed 方法 ***
// SimpleTokenCache 现在是纯数据结构，所有访问由 TokenManager.mutex 保护
// set 操作：直接通过 tm.cache.tokens[key] = value 完成
// updateLastUsed 操作：已合并到 getBestToken 方法中

// CalculateAvailableCount 计算可用次数 (基于CREDIT资源类型，返回浮点精度)
func CalculateAvailableCount(usage *types.UsageLimits) float64 {
	for _, breakdown := range usage.UsageBreakdownList {
		if breakdown.ResourceType == "CREDIT" {
			var totalAvailable float64

			// 优先使用免费试用额度 (如果存在且处于ACTIVE状态)
			if breakdown.FreeTrialInfo != nil && breakdown.FreeTrialInfo.FreeTrialStatus == "ACTIVE" {
				freeTrialAvailable := breakdown.FreeTrialInfo.UsageLimitWithPrecision - breakdown.FreeTrialInfo.CurrentUsageWithPrecision
				totalAvailable += freeTrialAvailable
			}

			// 加上基础额度
			baseAvailable := breakdown.UsageLimitWithPrecision - breakdown.CurrentUsageWithPrecision
			totalAvailable += baseAvailable

			if totalAvailable < 0 {
				return 0.0
			}
			return totalAvailable
		}
	}
	return 0.0
}

// generateConfigOrder 生成token配置的顺序
func generateConfigOrder(configs []AuthConfig) []string {
	var order []string

	for i := range configs {
		// 使用索引生成cache key，与refreshCache中的逻辑保持一致
		cacheKey := fmt.Sprintf(config.TokenCacheKeyFormat, i)
		order = append(order, cacheKey)
	}

	logger.Debug("生成配置顺序",
		logger.Int("config_count", len(configs)),
		logger.Any("order", order))

	return order
}

// AddConfig 动态添加新的认证配置
func (tm *TokenManager) AddConfig(cfg AuthConfig) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	// 添加到配置列表
	tm.configs = append(tm.configs, cfg)

	// 更新配置顺序
	newIndex := len(tm.configs) - 1
	cacheKey := fmt.Sprintf(config.TokenCacheKeyFormat, newIndex)
	tm.configOrder = append(tm.configOrder, cacheKey)

	// 异步刷新新添加的token（不阻塞）
	if !cfg.Disabled {
		go tm.refreshSingleTokenAsync(newIndex, cfg)
	}

	logger.Info("新配置已添加，正在异步刷新",
		logger.String("cache_key", cacheKey),
		logger.String("auth_type", cfg.AuthType))
}

// RemoveConfig 动态移除认证配置
// 保留现有缓存，只移除指定索引的配置
func (tm *TokenManager) RemoveConfig(index int) error {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	if index < 0 || index >= len(tm.configs) {
		return fmt.Errorf("无效的索引: %d", index)
	}

	// 移除配置
	tm.configs = append(tm.configs[:index], tm.configs[index+1:]...)

	// 移除对应的缓存
	oldCacheKey := fmt.Sprintf(config.TokenCacheKeyFormat, index)
	delete(tm.cache.tokens, oldCacheKey)
	delete(tm.exhausted, oldCacheKey)

	// 重建配置顺序和缓存键映射
	// 需要将索引大于 index 的缓存键重新映射
	newCache := make(map[string]*CachedToken)
	newOrder := make([]string, 0, len(tm.configs))
	newExhausted := make(map[string]bool)

	for i := range tm.configs {
		newCacheKey := fmt.Sprintf(config.TokenCacheKeyFormat, i)
		newOrder = append(newOrder, newCacheKey)

		// 计算旧的缓存键
		oldIdx := i
		if i >= index {
			oldIdx = i + 1 // 原来的索引
		}
		oldKey := fmt.Sprintf(config.TokenCacheKeyFormat, oldIdx)

		// 迁移缓存
		if cached, exists := tm.cache.tokens[oldKey]; exists {
			newCache[newCacheKey] = cached
		}
		// 迁移耗尽标记
		if tm.exhausted[oldKey] {
			newExhausted[newCacheKey] = true
		}
	}

	tm.cache.tokens = newCache
	tm.configOrder = newOrder
	tm.exhausted = newExhausted

	// 调整当前索引
	if tm.currentIndex >= len(tm.configs) {
		tm.currentIndex = 0
	}

	logger.Info("配置已移除",
		logger.Int("removed_index", index),
		logger.Int("remaining_configs", len(tm.configs)))

	return nil
}

// RefreshSingleTokenByIndex 刷新指定索引的 Token（公开方法，用于手动刷新）
func (tm *TokenManager) RefreshSingleTokenByIndex(index int) error {
	tm.mutex.RLock()
	if index < 0 || index >= len(tm.configs) {
		tm.mutex.RUnlock()
		return fmt.Errorf("无效的索引: %d", index)
	}
	cfg := tm.configs[index]
	tm.mutex.RUnlock()

	if cfg.Disabled {
		return fmt.Errorf("该配置已禁用")
	}

	// 异步刷新
	go tm.refreshSingleTokenAsync(index, cfg)

	logger.Info("已触发单个Token刷新",
		logger.Int("index", index),
		logger.String("auth_type", cfg.AuthType))

	return nil
}

// RefreshAllTokens 刷新所有 Token（公开方法，用于手动刷新全部）
// 分批异步刷新，每个 Token 间隔 500ms
func (tm *TokenManager) RefreshAllTokens() {
	tm.mutex.RLock()
	configs := make([]AuthConfig, len(tm.configs))
	copy(configs, tm.configs)
	tm.mutex.RUnlock()

	logger.Info("开始刷新所有Token", logger.Int("total", len(configs)))

	// 异步分批刷新
	go func() {
		const refreshInterval = 500 * time.Millisecond

		for i, cfg := range configs {
			if cfg.Disabled {
				continue
			}

			tm.refreshSingleTokenAsync(i, cfg)

			// 如果不是最后一个，等待间隔
			if i < len(configs)-1 {
				time.Sleep(refreshInterval)
			}
		}

		logger.Info("所有Token刷新完成", logger.Int("total", len(configs)))
	}()
}
