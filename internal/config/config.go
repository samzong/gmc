package config

import (
	"fmt"
	"os"
	"path/filepath"
	
	"github.com/spf13/viper"
)

// Config 表示应用程序配置
type Config struct {
	Role    string `mapstructure:"role"`
	Model   string `mapstructure:"model"`
	APIKey  string `mapstructure:"api_key"`
	APIBase string `mapstructure:"api_base"`
}

// 默认配置值
const (
	DefaultRole       = "全栈工程师"
	DefaultModel      = "gpt-3.5-turbo"
	DefaultConfigName = ".gma"
)

// 预设的角色列表，但用户可以自定义其他角色
var suggestedRoles = []string{
	"前端工程师",
	"后端工程师",
	"DevOps工程师",
	"全栈工程师",
	"Markdown工程师",
}

// 预设的模型列表，但用户可以自定义其他模型
var suggestedModels = []string{
	"gpt-3.5-turbo",
	"gpt-4",
	"gpt-4-turbo",
}

// InitConfig 初始化配置
func InitConfig(cfgFile string) {
	if cfgFile != "" {
		// 使用命令行标志中指定的配置文件
		viper.SetConfigFile(cfgFile)
	} else {
		// 查找用户主目录
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Println("错误: 无法找到主目录:", err)
			os.Exit(1)
		}

		// 在主目录中搜索配置文件
		viper.AddConfigPath(home)
		viper.SetConfigName(DefaultConfigName)
		viper.SetConfigType("yaml")
	}

	// 设置默认值
	viper.SetDefault("role", DefaultRole)
	viper.SetDefault("model", DefaultModel)
	viper.SetDefault("api_key", "")
	viper.SetDefault("api_base", "")

	// 读取环境变量
	viper.AutomaticEnv()

	// 尝试读取配置文件
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// 配置文件不存在，静默创建
			// 确保目录存在
			var configDir string
			if cfgFile != "" {
				configDir = filepath.Dir(cfgFile)
			} else {
				home, _ := os.UserHomeDir()
				configDir = home
			}
			
			if err := os.MkdirAll(configDir, 0755); err != nil {
				fmt.Printf("错误: 无法创建配置目录: %v\n", err)
				return
			}
			
			// 使用WriteConfigAs写入配置到指定路径
			configPath := ""
			if cfgFile != "" {
				configPath = cfgFile
			} else {
				home, _ := os.UserHomeDir()
				configPath = filepath.Join(home, DefaultConfigName+".yaml")
			}
			
			if err := viper.WriteConfigAs(configPath); err != nil {
				fmt.Printf("错误: 无法写入配置文件: %v\n", err)
				return
			}
		} else {
			// 其他错误
			fmt.Printf("错误: 无法读取配置文件: %v\n", err)
		}
	}
}

// GetConfig 获取当前配置
func GetConfig() *Config {
	cfg := &Config{}
	if err := viper.Unmarshal(cfg); err != nil {
		fmt.Println("错误: 无法解析配置:", err)
		return &Config{
			Role:    DefaultRole,
			Model:   DefaultModel,
			APIKey:  "",
			APIBase: "",
		}
	}
	return cfg
}

// SaveConfig 保存当前配置
func SaveConfig() error {
	return viper.WriteConfig()
}

// IsValidRole 检查角色是否为建议角色之一（但任何非空字符串都是有效的）
func IsValidRole(role string) bool {
	if role == "" {
		return false
	}
	return true
}

// IsValidModel 检查模型是否为建议模型之一（但任何非空字符串都是有效的）
func IsValidModel(model string) bool {
	if model == "" {
		return false
	}
	return true
}

// GetSuggestedRoles 获取所有建议角色
func GetSuggestedRoles() []string {
	return suggestedRoles
}

// GetSuggestedModels 获取所有建议模型
func GetSuggestedModels() []string {
	return suggestedModels
} 