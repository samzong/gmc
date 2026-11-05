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
	credentials = credentialState{}

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
		if !errors.As(err, &notFoundErr) && !os.IsNotExist(err) {
			return fmt.Errorf("failed to read configuration file: %w", err)
		}
	} else if err := enforceConfigFilePermissions(configPath); err != nil {
		fmt.Fprintf(credentialWarningWriter, "gmc: cannot secure configuration file permissions: %v\n", err)
	}

	loadRepoLayer(explicit)

	return nil
}

func getSettings() (*Config, error) {
	cfg := defaultConfig()
	if err := viper.Unmarshal(cfg); err != nil {
		return cfg, fmt.Errorf("failed to parse configuration: %w", err)
	}
	applyRepoLayer(cfg)
	return cfg, nil
}

func GetConfig() (*Config, error) {
	cfg, err := getSettings()
	if err != nil {
		return cfg, err
	}
	if key := os.Getenv("GMC_API_KEY"); key != "" {
		cfg.APIKey = key
	} else {
		warning := loadAPIKey()
		if credentials.key != "" {
			cfg.APIKey = credentials.key
		} else if cfg.APIKey != "" && configFilePath != "" {
			if warning != "" {
				warning += ". "
			}
			warning += fmt.Sprintf("API key is stored in plaintext in %q. "+
				"Run gmc config set apikey to move it to OS credential storage", configFilePath)
		}
		if warning != "" {
			warnCredential(warning)
		}
	}
	return cfg, nil
}

func MustGetSettings() *Config {
	cfg, err := getSettings()
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
	if err := writeConfig(); err != nil {
		return err
	}
	return enforceConfigFilePermissions(configFilePath)
}

func SetConfigValue(key string, value any) error {
	if strings.EqualFold(key, "api_key") {
		return storeAPIKey(fmt.Sprint(value))
	}
	viper.Set(key, value)
	return nil
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
