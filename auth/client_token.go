package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"kiro2api/logger"
)

// ClientToken 客户端认证令牌
type ClientToken struct {
	Token     string    `json:"token"`              // 令牌值
	Name      string    `json:"name,omitempty"`     // 可选名称/标签
	Disabled  bool      `json:"disabled,omitempty"` // 是否禁用
	CreatedAt time.Time `json:"createdAt"`          // 创建时间
}

// ClientTokenStats 客户端令牌运行时统计
type ClientTokenStats struct {
	Token        string    `json:"token"`        // 令牌预览（脱敏）
	Name         string    `json:"name"`         // 名称
	Disabled     bool      `json:"disabled"`     // 是否禁用
	CreatedAt    time.Time `json:"createdAt"`    // 创建时间
	RequestCount int64     `json:"requestCount"` // 请求次数
	LastUsedAt   *time.Time `json:"lastUsedAt"`  // 最后使用时间（可能为空）
}

// ClientTokenManager 客户端令牌管理器
type ClientTokenManager struct {
	mu           sync.RWMutex
	tokens       []ClientToken
	stats        map[string]*tokenStats // key: token value
	configFile   string
}

// tokenStats 内部统计结构
type tokenStats struct {
	requestCount int64
	lastUsedAt   time.Time
}

const (
	clientTokenConfigFile = "client_tokens.json"
)

// NewClientTokenManager 创建客户端令牌管理器
func NewClientTokenManager() (*ClientTokenManager, error) {
	manager := &ClientTokenManager{
		tokens: []ClientToken{},
		stats:  make(map[string]*tokenStats),
	}

	// 确定配置文件路径
	manager.configFile = clientTokenConfigFile

	// 尝试加载配置
	if err := manager.loadConfig(); err != nil {
		logger.Warn("加载客户端令牌配置失败，将使用空配置", logger.Err(err))
	}

	// 兼容：如果没有配置文件，尝试从环境变量加载
	if len(manager.tokens) == 0 {
		if envToken := os.Getenv("KIRO_CLIENT_TOKEN"); envToken != "" {
			manager.tokens = append(manager.tokens, ClientToken{
				Token:     envToken,
				Name:      "环境变量导入",
				CreatedAt: time.Now(),
			})
			logger.Info("从环境变量导入客户端令牌")
		}
	}

	logger.Info("ClientTokenManager 初始化完成",
		logger.Int("token_count", len(manager.tokens)))

	return manager, nil
}

// loadConfig 从文件加载配置
func (m *ClientTokenManager) loadConfig() error {
	data, err := os.ReadFile(m.configFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 文件不存在不是错误
		}
		return err
	}

	var tokens []ClientToken
	if err := json.Unmarshal(data, &tokens); err != nil {
		return fmt.Errorf("解析配置文件失败: %w", err)
	}

	m.tokens = tokens
	return nil
}

// saveConfig 保存配置到文件
func (m *ClientTokenManager) saveConfig() error {
	data, err := json.MarshalIndent(m.tokens, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	// 确保目录存在
	dir := filepath.Dir(m.configFile)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建目录失败: %w", err)
		}
	}

	if err := os.WriteFile(m.configFile, data, 0600); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	return nil
}

// ValidateToken 验证令牌是否有效，并记录使用
func (m *ClientTokenManager) ValidateToken(token string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, t := range m.tokens {
		if t.Token == token && !t.Disabled {
			// 更新统计
			if m.stats[token] == nil {
				m.stats[token] = &tokenStats{}
			}
			m.stats[token].requestCount++
			m.stats[token].lastUsedAt = time.Now()
			return true
		}
	}
	return false
}

// HasTokens 检查是否有可用的令牌
func (m *ClientTokenManager) HasTokens() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.tokens) > 0
}

// GetAllStats 获取所有令牌的统计信息
func (m *ClientTokenManager) GetAllStats() []ClientTokenStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]ClientTokenStats, 0, len(m.tokens))
	for _, t := range m.tokens {
		stat := ClientTokenStats{
			Token:     t.Token, // 返回完整令牌，前端负责显示/隐藏
			Name:      t.Name,
			Disabled:  t.Disabled,
			CreatedAt: t.CreatedAt,
		}

		if s, ok := m.stats[t.Token]; ok {
			stat.RequestCount = s.requestCount
			if !s.lastUsedAt.IsZero() {
				stat.LastUsedAt = &s.lastUsedAt
			}
		}

		result = append(result, stat)
	}
	return result
}

// AddToken 添加新令牌
func (m *ClientTokenManager) AddToken(token, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if token == "" {
		return fmt.Errorf("令牌不能为空")
	}

	// 检查是否已存在
	for _, t := range m.tokens {
		if t.Token == token {
			return fmt.Errorf("令牌已存在")
		}
	}

	newToken := ClientToken{
		Token:     token,
		Name:      name,
		CreatedAt: time.Now(),
	}

	// 保存旧配置用于回滚
	oldTokens := make([]ClientToken, len(m.tokens))
	copy(oldTokens, m.tokens)

	m.tokens = append(m.tokens, newToken)

	// 持久化
	if err := m.saveConfig(); err != nil {
		m.tokens = oldTokens
		return fmt.Errorf("保存配置失败: %w", err)
	}

	logger.Info("添加客户端令牌",
		logger.String("name", name),
		logger.Int("total_count", len(m.tokens)))

	return nil
}

// RemoveToken 根据索引移除令牌
func (m *ClientTokenManager) RemoveToken(index int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if index < 0 || index >= len(m.tokens) {
		return fmt.Errorf("无效的索引: %d", index)
	}

	// 保存旧配置用于回滚
	oldTokens := make([]ClientToken, len(m.tokens))
	copy(oldTokens, m.tokens)

	// 删除统计
	delete(m.stats, m.tokens[index].Token)

	// 移除令牌
	m.tokens = append(m.tokens[:index], m.tokens[index+1:]...)

	// 持久化
	if err := m.saveConfig(); err != nil {
		m.tokens = oldTokens
		return fmt.Errorf("保存配置失败: %w", err)
	}

	logger.Info("移除客户端令牌",
		logger.Int("removed_index", index),
		logger.Int("remaining_count", len(m.tokens)))

	return nil
}

// ToggleToken 切换令牌启用/禁用状态
func (m *ClientTokenManager) ToggleToken(index int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if index < 0 || index >= len(m.tokens) {
		return fmt.Errorf("无效的索引: %d", index)
	}

	m.tokens[index].Disabled = !m.tokens[index].Disabled

	// 持久化
	if err := m.saveConfig(); err != nil {
		m.tokens[index].Disabled = !m.tokens[index].Disabled // 回滚
		return fmt.Errorf("保存配置失败: %w", err)
	}

	logger.Info("切换客户端令牌状态",
		logger.Int("index", index),
		logger.Bool("disabled", m.tokens[index].Disabled))

	return nil
}

// GetTokenCount 获取令牌数量
func (m *ClientTokenManager) GetTokenCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.tokens)
}
