package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Role           string `mapstructure:"role"`
	Model          string `mapstructure:"model"`
	APIKey         string `mapstructure:"api_key"`
	APIBase        string `mapstructure:"api_base"`
	PromptTemplate string `mapstructure:"prompt_template"`
	EnableEmoji    bool   `mapstructure:"enable_emoji"`
}

const (
	DefaultRole           = "Developer"
	DefaultModel          = "gpt-3.5-turbo"
	DefaultConfigName     = "config"
	DefaultConfigDir      = "gmc"
	LegacyConfigName      = ".gmc"
	DefaultPromptTemplate = "default"
	EnvPrefix             = "GMC"
)

var configFilePath string

func FilePath() string {
	return configFilePath
}

func getConfigPath(cfgFile string) (string, bool, error) {
	if cfgFile != "" {
		return cfgFile, true, nil
	}

	if envConfig := os.Getenv("GMC_CONFIG"); envConfig != "" {
		return envConfig, true, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", false, fmt.Errorf("failed to find home directory: %w", err)
	}

	xdgConfigHome := os.Getenv("XDG_CONFIG_HOME")
	if xdgConfigHome == "" {
		xdgConfigHome = filepath.Join(home, ".config")
	}

	xdgConfigPath := filepath.Join(xdgConfigHome, DefaultConfigDir, DefaultConfigName+".yaml")

	if _, err := os.Stat(xdgConfigPath); err == nil {
		return xdgConfigPath, false, nil
	}

	legacyPath := filepath.Join(home, LegacyConfigName+".yaml")
	if _, err := os.Stat(legacyPath); err == nil {
		return legacyPath, false, nil
	}

	return xdgConfigPath, false, nil
}

func InitConfig(cfgFile string) error {
	configPath, explicit, err := getConfigPath(cfgFile)
	if err != nil {
		return err
	}
	configFilePath = configPath
	repo = repoLayer{}

	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	viper.SetDefault("role", DefaultRole)
	viper.SetDefault("model", DefaultModel)
	viper.SetDefault("api_key", "")
	viper.SetDefault("api_base", "")
	viper.SetDefault("prompt_template", DefaultPromptTemplate)
	viper.SetDefault("enable_emoji", false)

	viper.SetEnvPrefix(EnvPrefix)
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
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
		} else {
			return fmt.Errorf("failed to read configuration file: %w", err)
		}
	}
	if err := enforceConfigFilePermissions(configFilePath); err != nil {
		return err
	}

	loadRepoLayer(explicit)

	return nil
}

func GetConfig() (*Config, error) {
	cfg := defaultConfig()
	if err := viper.Unmarshal(cfg); err != nil {
		return cfg, fmt.Errorf("failed to parse configuration: %w", err)
	}
	applyRepoLayer(cfg)
	return cfg, nil
}

func MustGetConfig() *Config {
	cfg, err := GetConfig()
	if err != nil {
		return defaultConfig()
	}
	return cfg
}

func defaultConfig() *Config {
	return &Config{
		Role:           DefaultRole,
		Model:          DefaultModel,
		PromptTemplate: DefaultPromptTemplate,
	}
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
