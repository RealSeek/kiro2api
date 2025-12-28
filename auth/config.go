package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"kiro2api/logger"
)

// AuthConfig 简化的认证配置
type AuthConfig struct {
	AuthType     string `json:"auth"`
	RefreshToken string `json:"refreshToken"`
	ClientID     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
	Disabled     bool   `json:"disabled,omitempty"`
}

// 认证方法常量
const (
	AuthMethodSocial = "Social"
	AuthMethodIdC    = "IdC"
)

// 默认配置文件名
const DefaultConfigFileName = "auth_config.json"

// configFilePath 记录当前使用的配置文件路径（用于持久化）
var (
	configFilePath string
	configMutex    sync.RWMutex
)

// loadConfigs 从环境变量或默认配置文件加载配置
// 配置加载优先级: 环境变量文件 > 默认配置文件 > 环境变量 JSON
func loadConfigs() ([]AuthConfig, error) {
	// 检测并警告弃用的环境变量
	deprecatedVars := []string{
		"REFRESH_TOKEN",
		"AWS_REFRESHTOKEN",
		"IDC_REFRESH_TOKEN",
		"BULK_REFRESH_TOKENS",
	}

	for _, envVar := range deprecatedVars {
		if os.Getenv(envVar) != "" {
			logger.Warn("检测到已弃用的环境变量",
				logger.String("变量名", envVar),
				logger.String("迁移说明", "请迁移到KIRO_AUTH_TOKEN的JSON格式"))
			logger.Warn("迁移示例",
				logger.String("新格式", `KIRO_AUTH_TOKEN='[{"auth":"Social","refreshToken":"your_token"}]'`))
		}
	}

	// 获取环境变量
	jsonData := os.Getenv("KIRO_AUTH_TOKEN")

	var configData string
	var loadedFromFile bool

	// 配置加载优先级: 环境变量文件 > 默认配置文件 > 环境变量 JSON
	if jsonData != "" {
		// 优先尝试从文件加载
		if fileInfo, err := os.Stat(jsonData); err == nil && !fileInfo.IsDir() {
			// 是文件，读取文件内容
			content, err := os.ReadFile(jsonData)
			if err != nil {
				return nil, fmt.Errorf("读取配置文件失败: %w\n配置文件路径: %s", err, jsonData)
			}
			configData = string(content)
			setConfigFilePath(jsonData)
			loadedFromFile = true
			logger.Info("从环境变量指定的文件加载认证配置", logger.String("文件路径", jsonData))
		} else {
			// 不是文件，作为JSON字符串处理
			configData = jsonData
			logger.Debug("从环境变量加载JSON配置")
		}
	}

	// 如果环境变量未设置或不是文件，尝试默认配置文件
	if configData == "" || (!loadedFromFile && jsonData != "") {
		defaultPath := getDefaultConfigPath()
		if fileInfo, err := os.Stat(defaultPath); err == nil && !fileInfo.IsDir() {
			content, err := os.ReadFile(defaultPath)
			if err != nil {
				logger.Warn("读取默认配置文件失败", logger.Err(err))
			} else {
				configData = string(content)
				setConfigFilePath(defaultPath)
				loadedFromFile = true
				logger.Info("从默认配置文件加载认证配置", logger.String("文件路径", defaultPath))
			}
		}
	}

	// 如果仍然没有配置数据，允许空配置启动
	if configData == "" {
		// 设置默认配置文件路径，用于后续持久化
		setConfigFilePath(getDefaultConfigPath())
		logger.Info("未找到认证配置，将使用空配置启动（可通过API添加账号）",
			logger.String("持久化路径", getConfigFilePath()))
		return []AuthConfig{}, nil
	}

	// 解析JSON配置
	configs, err := parseJSONConfig(configData)
	if err != nil {
		return nil, fmt.Errorf("解析配置失败: %w\n"+
			"请检查JSON格式是否正确\n"+
			"示例: [{\"auth\":\"Social\",\"refreshToken\":\"token1\"}]", err)
	}

	// 允许空配置
	if len(configs) == 0 {
		logger.Info("配置文件为空，将使用空配置启动")
		return []AuthConfig{}, nil
	}

	// processConfigs 只过滤无效配置，保留 disabled 配置用于持久化
	validConfigs := processConfigsForRuntime(configs)

	logger.Info("成功加载认证配置",
		logger.Int("总配置数", len(configs)),
		logger.Int("有效配置数", len(validConfigs)),
		logger.Bool("从文件加载", loadedFromFile))

	return validConfigs, nil
}

// getDefaultConfigPath 获取默认配置文件路径
func getDefaultConfigPath() string {
	// 优先使用当前工作目录
	if cwd, err := os.Getwd(); err == nil {
		return filepath.Join(cwd, DefaultConfigFileName)
	}
	return DefaultConfigFileName
}

// setConfigFilePath 设置配置文件路径
func setConfigFilePath(path string) {
	configMutex.Lock()
	defer configMutex.Unlock()
	configFilePath = path
}

// getConfigFilePath 获取配置文件路径
func getConfigFilePath() string {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return configFilePath
}

// SaveConfigs 保存配置到文件
func SaveConfigs(configs []AuthConfig) error {
	filePath := getConfigFilePath()
	if filePath == "" {
		filePath = getDefaultConfigPath()
		setConfigFilePath(filePath)
	}

	configMutex.Lock()
	defer configMutex.Unlock()

	// 格式化 JSON（带缩进，便于阅读）
	data, err := json.MarshalIndent(configs, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	logger.Info("配置已保存到文件",
		logger.String("文件路径", filePath),
		logger.Int("配置数量", len(configs)))

	return nil
}

// processConfigsForRuntime 处理配置用于运行时（过滤无效和禁用的配置）
func processConfigsForRuntime(configs []AuthConfig) []AuthConfig {
	var validConfigs []AuthConfig

	for _, config := range configs {
		// 验证必要字段
		if config.RefreshToken == "" {
			continue
		}

		// 设置默认认证类型
		if config.AuthType == "" {
			config.AuthType = AuthMethodSocial
		}

		// 验证IdC认证的必要字段
		if config.AuthType == AuthMethodIdC {
			if config.ClientID == "" || config.ClientSecret == "" {
				continue
			}
		}

		// 跳过禁用的配置
		if config.Disabled {
			continue
		}

		validConfigs = append(validConfigs, config)
	}

	return validConfigs
}

// GetConfigs 公开的配置获取函数，供其他包调用
func GetConfigs() ([]AuthConfig, error) {
	return loadConfigs()
}

// parseJSONConfig 解析JSON配置字符串
func parseJSONConfig(jsonData string) ([]AuthConfig, error) {
	var configs []AuthConfig

	// 尝试解析为数组
	if err := json.Unmarshal([]byte(jsonData), &configs); err != nil {
		// 尝试解析为单个对象
		var single AuthConfig
		if err := json.Unmarshal([]byte(jsonData), &single); err != nil {
			return nil, fmt.Errorf("JSON格式无效: %w", err)
		}
		configs = []AuthConfig{single}
	}

	return configs, nil
}
