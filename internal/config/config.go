package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type Config struct {
	Role           string `mapstructure:"role"`
	Model          string `mapstructure:"model"`
	APIKey         string `mapstructure:"api_key"`
	APIBase        string `mapstructure:"api_base"`
	PromptTemplate string `mapstructure:"prompt_template"`
	PromptsDir     string `mapstructure:"prompts_dir"`
}

const (
	DefaultRole           = "Developer"
	DefaultModel          = "gpt-3.5-turbo"
	DefaultConfigName     = ".gmc"
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

func InitConfig(cfgFile string) error {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to find home directory: %w", err)
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
	defaultPromptsDir := filepath.Join(home, ".gmc", "prompts")
	viper.SetDefault("prompts_dir", defaultPromptsDir)

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
				return fmt.Errorf("failed to create configuration directory: %w", err)
			}

			configPath := ""
			if cfgFile != "" {
				configPath = cfgFile
			} else {
				home, _ := os.UserHomeDir()
				configPath = filepath.Join(home, DefaultConfigName+".yaml")
			}

			if err := viper.WriteConfigAs(configPath); err != nil {
				return fmt.Errorf("failed to write configuration file: %w", err)
			}
		} else {
			return fmt.Errorf("failed to read configuration file: %w", err)
		}
	}

	promptsDir := viper.GetString("prompts_dir")
	if promptsDir != "" {
		if err := os.MkdirAll(promptsDir, 0755); err != nil {
			fmt.Printf("Warning: Failed to create prompt directory %s: %v\n", promptsDir, err)
		}
	}

	return nil
}

func GetConfig() *Config {
	cfg := &Config{}
	if err := viper.Unmarshal(cfg); err != nil {
		fmt.Println("Error: Failed to parse configuration:", err)
		return &Config{
			Role:           DefaultRole,
			Model:          DefaultModel,
			APIKey:         "",
			APIBase:        "",
			PromptTemplate: DefaultPromptTemplate,
			PromptsDir:     filepath.Join(os.Getenv("HOME"), ".gmc", "prompts"),
		}
	}
	return cfg
}

func SaveConfig() error {
	return viper.WriteConfig()
}

func SetConfigValue(key string, value any) {
	viper.Set(key, value)
}

func IsValidRole(role string) bool {
	return role != ""
}

func IsValidModel(model string) bool {
	return model != ""
}

func GetSuggestedRoles() []string {
	return suggestedRoles
}

func GetSuggestedModels() []string {
	return suggestedModels
}
