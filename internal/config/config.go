package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/viper"
)

type Config struct {
	Role           string `mapstructure:"role"`
	Model          string `mapstructure:"model"`
	APIKey         string `mapstructure:"api_key"`
	APIBase        string `mapstructure:"api_base"`
	PromptTemplate string `mapstructure:"prompt_template"`
	PromptsDir     string `mapstructure:"prompts_dir"`
	EnableEmoji    bool   `mapstructure:"enable_emoji"`
}

const (
	DefaultRole           = "Developer"
	DefaultModel          = "gpt-3.5-turbo"
	DefaultConfigName     = ".gmc"
	DefaultPromptTemplate = "default"
)

var configFilePath string

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
		configFilePath = cfgFile
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to find home directory: %w", err)
		}

		viper.AddConfigPath(home)
		viper.SetConfigName(DefaultConfigName)
		viper.SetConfigType("yaml")
		configFilePath = filepath.Join(home, DefaultConfigName+".yaml")
	}

	viper.SetDefault("role", DefaultRole)
	viper.SetDefault("model", DefaultModel)
	viper.SetDefault("api_key", "")
	viper.SetDefault("api_base", "")
	viper.SetDefault("prompt_template", DefaultPromptTemplate)
	viper.SetDefault("enable_emoji", false)

	home, _ := os.UserHomeDir()
	defaultPromptsDir := filepath.Join(home, ".gmc", "prompts")
	viper.SetDefault("prompts_dir", defaultPromptsDir)

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		var notFoundErr viper.ConfigFileNotFoundError
		if errors.As(err, &notFoundErr) || os.IsNotExist(err) {
			configDir := filepath.Dir(configFilePath)
			if err := os.MkdirAll(configDir, 0755); err != nil {
				return fmt.Errorf("failed to create configuration directory: %w", err)
			}

			if err := viper.WriteConfigAs(configFilePath); err != nil {
				return fmt.Errorf("failed to write configuration file: %w", err)
			}
			viper.SetConfigFile(configFilePath)
			if err := enforceConfigFilePermissions(configFilePath); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("failed to read configuration file: %w", err)
		}
	} else {
		if used := viper.ConfigFileUsed(); used != "" {
			configFilePath = used
		}
		if err := enforceConfigFilePermissions(configFilePath); err != nil {
			return err
		}
	}

	promptsDir := viper.GetString("prompts_dir")
	if promptsDir != "" {
		if err := os.MkdirAll(promptsDir, 0755); err != nil {
			fmt.Printf("Warning: Failed to create prompt directory %s: %v\n", promptsDir, err)
		}
	}

	// Merge repo-level config if exists (higher priority than home config)
	if repoConfig := findRepoConfig(); repoConfig != "" {
		if err := viper.MergeInConfig(); err == nil {
			// Re-read with repo config path for merge
			repoViper := viper.New()
			repoViper.SetConfigFile(repoConfig)
			if err := repoViper.ReadInConfig(); err == nil {
				for _, key := range repoViper.AllKeys() {
					viper.Set(key, repoViper.Get(key))
				}
			}
		}
	}

	return nil
}

// findRepoConfig searches for .gmc.yaml in the current working directory.
func findRepoConfig() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	repoConfigPath := filepath.Join(cwd, DefaultConfigName+".yaml")
	if _, err := os.Stat(repoConfigPath); err == nil {
		return repoConfigPath
	}
	return ""
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
			EnableEmoji:    false,
		}
	}
	return cfg
}

func SaveConfig() error {
	if err := viper.WriteConfig(); err != nil {
		return err
	}
	return enforceConfigFilePermissions(configFilePath)
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

func enforceConfigFilePermissions(path string) error {
	if path == "" || runtime.GOOS == "windows" {
		return nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat configuration file: %w", err)
	}

	const securePerm os.FileMode = 0o600
	if info.Mode().Perm() != securePerm {
		if err := os.Chmod(path, securePerm); err != nil {
			return fmt.Errorf("failed to set configuration file permissions: %w", err)
		}
	}

	updatedInfo, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to verify configuration file permissions: %w", err)
	}

	if updatedInfo.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("configuration file %s remains readable by other users (mode %04o)",
			path, updatedInfo.Mode().Perm())
	}

	return nil
}
