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
	lastRefresh  time.Time
	configOrder  []string        // 配置顺序
	currentIndex int             // 当前使用的token索引
	exhausted    map[string]bool // 已耗尽的token记录
	refreshing   bool            // 是否正在刷新
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

	logger.Info("TokenManager初始化（顺序选择策略）",
		logger.Int("config_count", len(configs)),
		logger.Int("config_order_count", len(configOrder)))

	return &TokenManager{
		cache:        NewSimpleTokenCache(config.TokenCacheTTL),
		configs:      configs,
		configOrder:  configOrder,
		currentIndex: 0,
		exhausted:    make(map[string]bool),
	}
}

// getBestToken 获取最优可用token
// 统一锁管理：所有操作在单一锁保护下完成，避免多次加锁/解锁
func (tm *TokenManager) getBestToken() (types.TokenInfo, error) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	// 检查是否需要刷新缓存（在锁内）
	if time.Since(tm.lastRefresh) > config.TokenCacheTTL {
		if err := tm.refreshCacheUnlocked(); err != nil {
			logger.Warn("刷新token缓存失败", logger.Err(err))
		}
	}

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
// 统一锁管理：所有操作在单一锁保护下完成
func (tm *TokenManager) GetBestTokenWithUsage() (*types.TokenWithUsage, error) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	// 检查是否需要刷新缓存（在锁内）
	if time.Since(tm.lastRefresh) > config.TokenCacheTTL {
		if err := tm.refreshCacheUnlocked(); err != nil {
			logger.Warn("刷新token缓存失败", logger.Err(err))
		}
	}

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
// 重构说明：从selectBestToken改为Unlocked后缀，明确锁约定
func (tm *TokenManager) selectBestTokenUnlocked() *CachedToken {
	// 调用者已持有 tm.mutex，无需额外加锁

	// 如果没有配置顺序，降级到按map遍历顺序
	if len(tm.configOrder) == 0 {
		for key, cached := range tm.cache.tokens {
			if time.Since(cached.CachedAt) <= tm.cache.ttl && cached.IsUsable() {
				logger.Debug("顺序策略选择token（无顺序配置）",
					logger.String("selected_key", key),
					logger.Float64("available_count", cached.Available))
				return cached
			}
		}
		return nil
	}

	// 从当前索引开始，找到第一个可用的token
	for attempts := 0; attempts < len(tm.configOrder); attempts++ {
		currentKey := tm.configOrder[tm.currentIndex]

		// 检查这个token是否存在且可用
		if cached, exists := tm.cache.tokens[currentKey]; exists {
			// 检查token是否过期
			if time.Since(cached.CachedAt) > tm.cache.ttl {
				tm.exhausted[currentKey] = true
				tm.currentIndex = (tm.currentIndex + 1) % len(tm.configOrder)
				continue
			}

			// 检查token是否可用
			if cached.IsUsable() {
				logger.Debug("顺序策略选择token",
					logger.String("selected_key", currentKey),
					logger.Int("index", tm.currentIndex),
					logger.Float64("available_count", cached.Available))
				return cached
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

// refreshCacheUnlocked 触发异步分批刷新token缓存
// 内部方法：调用者必须持有 tm.mutex
func (tm *TokenManager) refreshCacheUnlocked() error {
	// 如果已经在刷新中，跳过
	if tm.refreshing {
		logger.Debug("token缓存刷新已在进行中，跳过")
		return nil
	}

	tm.refreshing = true
	tm.lastRefresh = time.Now()

	// 启动异步分批刷新
	go tm.asyncBatchRefresh()

	return nil
}

// asyncBatchRefresh 异步分批刷新所有token
// 每个token刷新之间有间隔，避免短时间内发起过多请求
func (tm *TokenManager) asyncBatchRefresh() {
	defer func() {
		tm.mutex.Lock()
		tm.refreshing = false
		tm.mutex.Unlock()
	}()

	logger.Debug("开始异步分批刷新token缓存")

	// 获取配置快照（避免长时间持有锁）
	tm.mutex.RLock()
	configs := make([]AuthConfig, len(tm.configs))
	copy(configs, tm.configs)
	tm.mutex.RUnlock()

	// 分批刷新，每个token之间间隔500ms
	const refreshInterval = 500 * time.Millisecond

	for i, cfg := range configs {
		if cfg.Disabled {
			continue
		}

		// 刷新单个token
		tm.refreshSingleTokenAsync(i, cfg)

		// 如果不是最后一个，等待间隔
		if i < len(configs)-1 {
			time.Sleep(refreshInterval)
		}
	}

	logger.Debug("异步分批刷新token缓存完成",
		logger.Int("total_configs", len(configs)))
}

// refreshSingleTokenAsync 异步刷新单个token并更新缓存
func (tm *TokenManager) refreshSingleTokenAsync(index int, cfg AuthConfig) {
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
	cacheKey := fmt.Sprintf(config.TokenCacheKeyFormat, index)

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
