package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

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

type RepoConfigStatus struct {
	Path         string
	Ignored      []string
	IgnoredTotal int
	Skipped      bool
	Err          error
}

type repoLayer struct {
	RepoConfigStatus
	values map[string]any
}

var repo repoLayer

func RepoConfig() RepoConfigStatus {
	return repo.RepoConfigStatus
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
	repo.Path = path

	if explicit {
		repo.Skipped = true
		return
	}

	repoViper := viper.New()
	repoViper.SetConfigFile(path)
	if err := repoViper.ReadInConfig(); err != nil {
		repo.Err = fmt.Errorf("ignoring project config %s: %w", path, err)
		return
	}

	values := make(map[string]any)
	for _, key := range repoViper.AllKeys() {
		if !repoAllowedKeys[key] {
			repo.IgnoredTotal++
			if len(repo.Ignored) < maxIgnoredKeys {
				repo.Ignored = append(repo.Ignored, safeKeyName(key))
			}
			continue
		}
		values[key] = repoViper.Get(key)
	}
	sort.Strings(repo.Ignored)
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
