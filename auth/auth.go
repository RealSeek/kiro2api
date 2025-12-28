package auth

import (
	"fmt"
	"kiro2api/logger"
	"kiro2api/types"
)

// AuthService 认证服务（推荐使用依赖注入方式）
type AuthService struct {
	tokenManager *TokenManager
	configs      []AuthConfig
}

// NewAuthService 创建新的认证服务（推荐使用此方法而不是全局函数）
// 支持空配置启动，允许通过 API 动态添加账号
func NewAuthService() (*AuthService, error) {
	logger.Info("创建AuthService实例")

	// 加载配置
	configs, err := loadConfigs()
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %w", err)
	}

	// 允许空配置启动，后续可通过 API 添加账号
	if len(configs) == 0 {
		logger.Info("未找到token配置，将使用空配置启动（可通过API添加账号）")
		return &AuthService{
			tokenManager: NewTokenManager(configs),
			configs:      configs,
		}, nil
	}

	// 创建token管理器
	tokenManager := NewTokenManager(configs)

	// 预热第一个可用token
	_, warmupErr := tokenManager.getBestToken()
	if warmupErr != nil {
		logger.Warn("token预热失败", logger.Err(warmupErr))
	}

	logger.Info("AuthService创建完成", logger.Int("config_count", len(configs)))

	return &AuthService{
		tokenManager: tokenManager,
		configs:      configs,
	}, nil
}

// GetToken 获取可用的token
func (as *AuthService) GetToken() (types.TokenInfo, error) {
	if as.tokenManager == nil {
		return types.TokenInfo{}, fmt.Errorf("token管理器未初始化")
	}
	return as.tokenManager.getBestToken()
}

// GetTokenWithUsage 获取可用的token（包含使用信息）
func (as *AuthService) GetTokenWithUsage() (*types.TokenWithUsage, error) {
	if as.tokenManager == nil {
		return nil, fmt.Errorf("token管理器未初始化")
	}
	return as.tokenManager.GetBestTokenWithUsage()
}

// GetTokenManager 获取底层的TokenManager（用于高级操作）
func (as *AuthService) GetTokenManager() *TokenManager {
	return as.tokenManager
}

// GetConfigs 获取认证配置
func (as *AuthService) GetConfigs() []AuthConfig {
	return as.configs
}

// AddConfig 添加新的认证配置（自动持久化，失败时回滚）
func (as *AuthService) AddConfig(config AuthConfig) error {
	if config.RefreshToken == "" {
		return fmt.Errorf("RefreshToken 不能为空")
	}
	if config.AuthType == "" {
		config.AuthType = AuthMethodSocial
	}
	if config.AuthType == AuthMethodIdC {
		if config.ClientID == "" || config.ClientSecret == "" {
			return fmt.Errorf("IdC 认证需要 ClientID 和 ClientSecret")
		}
	}

	// 保存旧配置用于回滚
	oldConfigs := make([]AuthConfig, len(as.configs))
	copy(oldConfigs, as.configs)

	// 添加到内存配置
	as.configs = append(as.configs, config)
	as.tokenManager.AddConfig(config)

	// 持久化到文件
	if err := SaveConfigs(as.configs); err != nil {
		// 回滚内存状态
		as.configs = oldConfigs
		as.tokenManager = NewTokenManager(oldConfigs)
		logger.Error("持久化配置失败，已回滚",
			logger.Err(err),
			logger.Int("config_count", len(oldConfigs)))
		return fmt.Errorf("保存配置失败: %w", err)
	}

	logger.Info("添加新的认证配置",
		logger.String("auth_type", config.AuthType),
		logger.Int("total_configs", len(as.configs)))

	return nil
}

// RemoveConfig 根据索引移除配置（自动持久化，失败时回滚）
func (as *AuthService) RemoveConfig(index int) error {
	if index < 0 || index >= len(as.configs) {
		return fmt.Errorf("无效的索引: %d", index)
	}

	// 保存旧配置用于回滚
	oldConfigs := make([]AuthConfig, len(as.configs))
	copy(oldConfigs, as.configs)

	// 从内存中移除
	as.configs = append(as.configs[:index], as.configs[index+1:]...)

	// 使用 TokenManager 的 RemoveConfig 方法，保留现有缓存
	if err := as.tokenManager.RemoveConfig(index); err != nil {
		// 回滚内存状态
		as.configs = oldConfigs
		logger.Error("TokenManager移除配置失败，已回滚",
			logger.Err(err),
			logger.Int("config_count", len(oldConfigs)))
		return fmt.Errorf("移除配置失败: %w", err)
	}

	// 持久化到文件
	if err := SaveConfigs(as.configs); err != nil {
		// 回滚内存状态（需要重建 TokenManager）
		as.configs = oldConfigs
		as.tokenManager = NewTokenManager(oldConfigs)
		logger.Error("持久化配置失败，已回滚",
			logger.Err(err),
			logger.Int("config_count", len(oldConfigs)))
		return fmt.Errorf("保存配置失败: %w", err)
	}

	logger.Info("移除认证配置",
		logger.Int("removed_index", index),
		logger.Int("remaining_configs", len(as.configs)))

	return nil
}

// GetConfigCount 返回配置数量
func (as *AuthService) GetConfigCount() int {
	return len(as.configs)
}

// HasAvailableToken 检查是否有可用的 Token
func (as *AuthService) HasAvailableToken() bool {
	return len(as.configs) > 0
}
