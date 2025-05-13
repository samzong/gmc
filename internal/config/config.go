package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type Config struct {
	Role             string `mapstructure:"role"`
	Model            string `mapstructure:"model"`
	APIKey           string `mapstructure:"api_key"`
	APIBase          string `mapstructure:"api_base"`
	PromptTemplate   string `mapstructure:"prompt_template"`
	CustomPromptsDir string `mapstructure:"custom_prompts_dir"`
}

const (
	DefaultRole           = "Developer"
	DefaultModel          = "gpt-3.5-turbo"
	DefaultConfigName     = ".gma"
	DefaultPromptTemplate = "default"
)

var suggestedRoles = []string{
	"Developer",
	"Frontend Developer",
	"Backend Developer",
	"DevOps Engineer",
	"Full Stack Developer",
	"Markdown Engineer",
}

var suggestedModels = []string{
	"gpt-3.5-turbo",
	"gpt-4",
	"gpt-4-turbo",
}

func InitConfig(cfgFile string) {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Println("Error: Failed to find home directory:", err)
			os.Exit(1)
		}

		viper.AddConfigPath(home)
		viper.SetConfigName(DefaultConfigName)
		viper.SetConfigType("yaml")
	}

	viper.SetDefault("role", DefaultRole)
	viper.SetDefault("model", DefaultModel)
	viper.SetDefault("api_key", "")
	viper.SetDefault("api_base", "")
	viper.SetDefault("prompt_template", DefaultPromptTemplate)

	home, _ := os.UserHomeDir()
	defaultPromptsDir := filepath.Join(home, ".gma", "prompts")
	viper.SetDefault("custom_prompts_dir", defaultPromptsDir)

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			var configDir string
			if cfgFile != "" {
				configDir = filepath.Dir(cfgFile)
			} else {
				home, _ := os.UserHomeDir()
				configDir = home
			}

			if err := os.MkdirAll(configDir, 0755); err != nil {
				fmt.Printf("Error: Failed to create configuration directory: %v\n", err)
				return
			}

			configPath := ""
			if cfgFile != "" {
				configPath = cfgFile
			} else {
				home, _ := os.UserHomeDir()
				configPath = filepath.Join(home, DefaultConfigName+".yaml")
			}

			if err := viper.WriteConfigAs(configPath); err != nil {
				fmt.Printf("Error: Failed to write configuration file: %v\n", err)
				return
			}
		} else {
			fmt.Printf("Error: Failed to read configuration file: %v\n", err)
		}
	}

	promptsDir := viper.GetString("custom_prompts_dir")
	if promptsDir != "" {
		if err := os.MkdirAll(promptsDir, 0755); err != nil {
			fmt.Printf("Warning: Failed to create custom prompt directory %s: %v\n", promptsDir, err)
		}
	}
}

func GetConfig() *Config {
	cfg := &Config{}
	if err := viper.Unmarshal(cfg); err != nil {
		fmt.Println("Error: Failed to parse configuration:", err)
		return &Config{
			Role:             DefaultRole,
			Model:            DefaultModel,
			APIKey:           "",
			APIBase:          "",
			PromptTemplate:   DefaultPromptTemplate,
			CustomPromptsDir: filepath.Join(os.Getenv("HOME"), ".gma", "prompts"),
		}
	}
	return cfg
}

func SaveConfig() error {
	return viper.WriteConfig()
}

func SetConfigValue(key string, value interface{}) {
	viper.Set(key, value)
}

func IsValidRole(role string) bool {
	if role == "" {
		return false
	}
	return true
}

func IsValidModel(model string) bool {
	if model == "" {
		return false
	}
	return true
}

func GetSuggestedRoles() []string {
	return suggestedRoles
}

func GetSuggestedModels() []string {
	return suggestedModels
}
