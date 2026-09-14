package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
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

var repoAllowedKeys = map[string]bool{
	"role":         true,
	"model":        true,
	"enable_emoji": true,
}

const maxIgnoredKeys = 10

func safeKeyName(key string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '_', r == '-', r == '.':
			return r
		default:
			return -1
		}
	}, key)

	if cleaned == "" {
		return "(unnamed key)"
	}
	const maxKeyLength = 64
	if len(cleaned) > maxKeyLength {
		cleaned = cleaned[:maxKeyLength] + "..."
	}
	return cleaned
}

type repoLayer struct {
	path    string
	values  map[string]any
	ignored []string
	total   int
	skipped bool
	err     error
}

var repo repoLayer

type RepoConfigStatus struct {
	Path         string
	Ignored      []string
	IgnoredTotal int
	Skipped      bool
	Err          error
}

func RepoConfig() RepoConfigStatus {
	return RepoConfigStatus{
		Path:         repo.path,
		Ignored:      repo.ignored,
		IgnoredTotal: repo.total,
		Skipped:      repo.skipped,
		Err:          repo.err,
	}
}

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
			if err := enforceConfigFilePermissions(configFilePath); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("failed to read configuration file: %w", err)
		}
	} else {
		if err := enforceConfigFilePermissions(configFilePath); err != nil {
			return err
		}
	}

	loadRepoLayer(explicit)

	return nil
}

func findRepoConfig() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	repoConfigPath := filepath.Join(cwd, LegacyConfigName+".yaml")
	if _, err := os.Stat(repoConfigPath); err == nil {
		return repoConfigPath
	}
	return ""
}

func loadRepoLayer(explicit bool) {
	path := findRepoConfig()
	if path == "" {
		return
	}
	repo.path = path

	if explicit {
		repo.skipped = true
		return
	}

	repoViper := viper.New()
	repoViper.SetConfigFile(path)
	if err := repoViper.ReadInConfig(); err != nil {
		repo.err = fmt.Errorf("ignoring project config %s: %w", path, err)
		return
	}

	values := make(map[string]any)
	for _, key := range repoViper.AllKeys() {
		if !repoAllowedKeys[key] {
			repo.total++
			if len(repo.ignored) < maxIgnoredKeys {
				repo.ignored = append(repo.ignored, safeKeyName(key))
			}
			continue
		}
		values[key] = repoViper.Get(key)
	}
	sort.Strings(repo.ignored)
	repo.values = values
}

func applyRepoLayer(cfg *Config) {
	for key, value := range repo.values {
		if envVarSet(key) {
			continue
		}

		switch key {
		case "role":
			if v := stringValue(value); v != "" {
				cfg.Role = v
			}
		case "model":
			if v := stringValue(value); v != "" {
				cfg.Model = v
			}
		case "enable_emoji":
			if v, ok := boolValue(value); ok {
				cfg.EnableEmoji = v
			}
		}
	}
}

func envKey(key string) string {
	return EnvPrefix + "_" + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
}

func envVarSet(key string) bool {
	_, ok := os.LookupEnv(envKey(key))
	return ok
}

func stringValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
}

func boolValue(value any) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(v))
		if err != nil {
			return false, false
		}
		return parsed, true
	default:
		return false, false
	}
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
		APIKey:         "",
		APIBase:        "",
		PromptTemplate: DefaultPromptTemplate,
		EnableEmoji:    false,
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
