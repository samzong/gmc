package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigFiles(t *testing.T) {
	for _, tt := range []struct {
		name    string
		content string
		want    *Config
	}{
		{"defaults", "", defaultConfig()},
		{"existing", "role: Senior Go Developer\nmodel: gpt-4\napi_key: existing-key\n" +
			"api_base: https://api.custom.com/v1\nprompt_template: /custom/prompt.yaml\n",
			&Config{Role: "Senior Go Developer", Model: "gpt-4", APIKey: "existing-key",
				APIBase: "https://api.custom.com/v1", PromptTemplate: "/custom/prompt.yaml"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := arrangeRepoConfig(t, tt.content, "")
			cfg := loadConfig(t, path)
			assert.Equal(t, tt.want, cfg)
			if runtime.GOOS != "windows" {
				info, err := os.Stat(path)
				require.NoError(t, err)
				assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
			}
		})
	}
}

func TestConfigCreationAndPaths(t *testing.T) {
	path := arrangeRepoConfig(t, "", "")
	require.NoError(t, os.Remove(path))
	t.Setenv("XDG_CONFIG_HOME", "")
	require.NoError(t, InitConfig(""))
	assert.FileExists(t, path)
	assert.Equal(t, path, FilePath())
	cfg, err := GetConfig()
	require.NoError(t, err)
	assert.Equal(t, defaultConfig(), cfg)
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
	legacy := filepath.Join(os.Getenv("HOME"), ".gmc.yaml")
	require.NoError(t, os.Rename(path, legacy))
	require.NoError(t, InitConfig(""))
	assert.Equal(t, legacy, FilePath())
	t.Setenv("GMC_CONFIG", path)
	require.NoError(t, InitConfig(""))
	assert.Equal(t, path, FilePath())
}

func TestInvalidConfig(t *testing.T) {
	path := arrangeRepoConfig(t, "model: [invalid yaml", "")
	require.ErrorContains(t, InitConfig(path), "failed to read configuration file")
	viper.Reset()
	viper.Set("enable_emoji", []string{"not", "a", "boolean"})
	_, err := GetConfig()
	require.Error(t, err)
	assert.Equal(t, defaultConfig(), MustGetConfig())
}

func TestSaveConfig(t *testing.T) {
	path := arrangeRepoConfig(t, "role: Original Role\nmodel: original-model\n", "")
	require.NoError(t, InitConfig(path))
	SetConfigValue("role", "Modified Role")
	SetConfigValue("model", "modified-model")
	require.NoError(t, SaveConfig())
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(content), "Modified Role")
	assert.Contains(t, string(content), "modified-model")
}

func TestConfigDirectoryError(t *testing.T) {
	arrangeRepoConfig(t, "", "")
	parent := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(parent, []byte("file"), 0o600))
	require.Error(t, InitConfig(filepath.Join(parent, "config.yaml")))
}

func TestConfigEnvironment(t *testing.T) {
	path := arrangeRepoConfig(t, "role: Config Role\nmodel: config-model\n", "")
	t.Setenv("GMC_ROLE", "Env Developer")
	t.Setenv("GMC_MODEL", "env-model")
	cfg := loadConfig(t, path)
	assert.Equal(t, "Env Developer", cfg.Role)
	assert.Equal(t, "env-model", cfg.Model)
}

func TestMissingHome(t *testing.T) {
	arrangeRepoConfig(t, "", "")
	t.Setenv("HOME", "")
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", "")
	}
	require.ErrorContains(t, InitConfig(""), "failed to find home directory")
}

func loadConfig(t *testing.T, path string) *Config {
	t.Helper()
	require.NoError(t, InitConfig(path))
	cfg, err := GetConfig()
	require.NoError(t, err)
	return cfg
}

func arrangeRepoConfig(t *testing.T, userYAML, repoYAML string) string {
	t.Helper()

	viper.Reset()
	t.Cleanup(func() { repo = repoLayer{} })

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	unsetEnv(t, "GMC_CONFIG", "GMC_ROLE", "GMC_MODEL", "GMC_API_KEY",
		"GMC_API_BASE", "GMC_PROMPT_TEMPLATE", "GMC_ENABLE_EMOJI")

	userConfig := filepath.Join(home, ".config", "gmc", "config.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(userConfig), 0o755))
	require.NoError(t, os.WriteFile(userConfig, []byte(userYAML), 0o600))

	repoDir := t.TempDir()
	if repoYAML != "" {
		require.NoError(t, os.WriteFile(filepath.Join(repoDir, ".gmc.yaml"), []byte(repoYAML), 0o644))
	}
	t.Chdir(repoDir)

	return userConfig
}

func unsetEnv(t *testing.T, names ...string) {
	t.Helper()

	for _, name := range names {
		if old, ok := os.LookupEnv(name); ok {
			t.Cleanup(func() { os.Setenv(name, old) })
		}
		os.Unsetenv(name)
	}
}

func TestRepoConfig_CannotSetCredentials(t *testing.T) {
	arrangeRepoConfig(t,
		"api_key: USER_KEY\napi_base: https://legit.example/v1\nprompt_template: user-template.yaml\n",
		"api_key: ATTACKER_KEY\napi_base: https://attacker.example/v1\nprompt_template: /repo/attacker.yaml\n")

	cfg := loadConfig(t, "")
	assert.Equal(t, "USER_KEY", cfg.APIKey)
	assert.Equal(t, "https://legit.example/v1", cfg.APIBase)
	assert.Equal(t, "user-template.yaml", cfg.PromptTemplate)

	info := RepoConfig()
	assert.Equal(t, []string{"api_base", "api_key", "prompt_template"}, info.Ignored)
	assert.NotEmpty(t, info.Path)
	assert.False(t, info.Skipped)
	assert.NoError(t, info.Err)
}

func TestRepoConfig_CannotRedirectTemplatePath(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "id_rsa")
	require.NoError(t, os.WriteFile(secret, []byte("SUPERSECRET_KEY_MATERIAL\n"), 0o600))

	arrangeRepoConfig(t, "prompt_template: "+filepath.Join(t.TempDir(), "mine.yaml")+"\n",
		"prompt_template: "+secret+"\n")

	cfg := loadConfig(t, "")
	assert.NotEqual(t, secret, cfg.PromptTemplate, "a repository must not choose the template path")
	assert.Contains(t, RepoConfig().Ignored, "prompt_template")
}

func TestRepoConfig_AppliesAllowedKeys(t *testing.T) {
	arrangeRepoConfig(t,
		"role: user-role\nmodel: user-model\n",
		"role: repo-role\nmodel: repo-model\nenable_emoji: true\n")

	cfg := loadConfig(t, "")
	assert.Equal(t, "repo-role", cfg.Role)
	assert.Equal(t, "repo-model", cfg.Model)
	assert.True(t, cfg.EnableEmoji)
	assert.Empty(t, RepoConfig().Ignored)
}

func TestRepoConfig_UnknownKeyIsIgnored(t *testing.T) {
	arrangeRepoConfig(t, "role: user-role\n", "role: repo-role\nsome_future_key: value\n")

	cfg := loadConfig(t, "")
	assert.Equal(t, "repo-role", cfg.Role)
	assert.Equal(t, []string{"some_future_key"}, RepoConfig().Ignored)
}

func TestRepoConfig_EnvironmentWinsOverRepoConfig(t *testing.T) {
	arrangeRepoConfig(t,
		"role: user-role\nmodel: user-model\n",
		"role: repo-role\nmodel: repo-model\n")

	t.Setenv("GMC_MODEL", "env-model")

	cfg := loadConfig(t, "")
	assert.Equal(t, "env-model", cfg.Model, "GMC_* must outrank the project config")
	assert.Equal(t, "repo-role", cfg.Role, "project config still overrides the user config file")
}

func TestRepoConfig_ExplicitConfigFileSkipsRepoConfig(t *testing.T) {
	arrangeRepoConfig(t, "model: user-model\n", "model: repo-model\nrole: repo-role\n")

	explicit := filepath.Join(t.TempDir(), "explicit.yaml")
	require.NoError(t, os.WriteFile(explicit, []byte("model: explicit-model\n"), 0o600))

	cfg := loadConfig(t, explicit)
	assert.Equal(t, "explicit-model", cfg.Model)
	assert.Equal(t, DefaultRole, cfg.Role)

	info := RepoConfig()
	assert.True(t, info.Skipped)
	assert.NotEmpty(t, info.Path)
}

func TestRepoConfig_NotPersistedBySaveConfig(t *testing.T) {
	userConfig := arrangeRepoConfig(t,
		"api_key: USER_KEY\nrole: user-role\nmodel: user-model\n",
		"api_base: https://attacker.example/v1\nrole: repo-role\nmodel: repo-model\n")

	require.NoError(t, InitConfig(""))

	SetConfigValue("model", "my-new-model")
	require.NoError(t, SaveConfig())

	saved, err := os.ReadFile(userConfig)
	require.NoError(t, err)
	assert.Contains(t, string(saved), "my-new-model")
	assert.Contains(t, string(saved), "USER_KEY")
	assert.NotContains(t, string(saved), "attacker.example")
	assert.NotContains(t, string(saved), "repo-role")
}

func TestRepoConfig_BrokenProjectConfigIsReported(t *testing.T) {
	arrangeRepoConfig(t, "model: user-model\n", "model: [unclosed\n")

	require.NoError(t, InitConfig(""))

	info := RepoConfig()
	require.Error(t, info.Err)
	assert.Contains(t, info.Err.Error(), "ignoring project config")

	cfg, err := GetConfig()
	require.NoError(t, err)
	assert.Equal(t, "user-model", cfg.Model)
}

func TestRepoConfig_AbsentProjectConfig(t *testing.T) {
	arrangeRepoConfig(t, "model: user-model\n", "")

	require.NoError(t, InitConfig(""))

	info := RepoConfig()
	assert.Empty(t, info.Path)
	assert.Empty(t, info.Ignored)
	assert.False(t, info.Skipped)
	assert.NoError(t, info.Err)
}
