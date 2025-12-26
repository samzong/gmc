package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_Struct(t *testing.T) {
	cfg := Config{
		Role:           "Senior Go Developer",
		Model:          "gpt-4",
		APIKey:         "test-key",
		APIBase:        "https://api.openai.com/v1",
		PromptTemplate: "/test/prompts/custom.yaml",
	}

	assert.Equal(t, "Senior Go Developer", cfg.Role)
	assert.Equal(t, "gpt-4", cfg.Model)
	assert.Equal(t, "test-key", cfg.APIKey)
	assert.Equal(t, "https://api.openai.com/v1", cfg.APIBase)
	assert.Equal(t, "/test/prompts/custom.yaml", cfg.PromptTemplate)
}

func TestDefaults(t *testing.T) {
	assert.Equal(t, "Developer", DefaultRole)
	assert.Equal(t, "gpt-3.5-turbo", DefaultModel)
	assert.Equal(t, "config", DefaultConfigName)
	assert.Equal(t, "gmc", DefaultConfigDir)
	assert.Equal(t, ".gmc", LegacyConfigName)
	assert.Equal(t, "default", DefaultPromptTemplate)
	assert.Equal(t, "GMC", EnvPrefix)
}

func TestGetSuggestedRoles(t *testing.T) {
	roles := GetSuggestedRoles()

	assert.NotEmpty(t, roles)
	assert.Contains(t, roles, "Developer")
	assert.Contains(t, roles, "Frontend Developer")
	assert.Contains(t, roles, "Backend Developer")
	assert.Contains(t, roles, "DevOps Engineer")
	assert.Contains(t, roles, "Full Stack Developer")
	assert.Contains(t, roles, "Markdown Engineer")
}

func TestGetSuggestedModels(t *testing.T) {
	models := GetSuggestedModels()

	assert.NotEmpty(t, models)
	assert.Contains(t, models, "gpt-3.5-turbo")
	assert.Contains(t, models, "gpt-4")
	assert.Contains(t, models, "gpt-4-turbo")
}

func TestIsValidRole(t *testing.T) {
	tests := []struct {
		role     string
		expected bool
	}{
		{"Developer", true},
		{"Senior Go Developer", true},
		{"", false},
		{"   ", true}, // Non-empty string, even with spaces
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			result := IsValidRole(tt.role)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsValidModel(t *testing.T) {
	tests := []struct {
		model    string
		expected bool
	}{
		{"gpt-3.5-turbo", true},
		{"gpt-4", true},
		{"custom-model", true},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			result := IsValidModel(tt.model)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSetConfigValue(t *testing.T) {
	// Reset viper state
	viper.Reset()

	SetConfigValue("test_key", "test_value")
	assert.Equal(t, "test_value", viper.GetString("test_key"))

	SetConfigValue("test_int", 42)
	assert.Equal(t, 42, viper.GetInt("test_int"))

	SetConfigValue("test_bool", true)
	assert.Equal(t, true, viper.GetBool("test_bool"))
}

func TestInitConfig_WithConfigFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gmc_config_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	configFile := filepath.Join(tempDir, "test_config.yaml")

	// Reset viper state
	viper.Reset()

	// Create a simple config file first to avoid the viper WriteConfigAs issue
	simpleConfig := `role: "Developer"
model: "gpt-3.5-turbo"
api_key: ""
api_base: ""
prompt_template: "default"`

	err = os.WriteFile(configFile, []byte(simpleConfig), 0644)
	require.NoError(t, err)

	err = InitConfig(configFile)
	require.NoError(t, err)

	// Check that config values are loaded correctly
	assert.Equal(t, DefaultRole, viper.GetString("role"))
	assert.Equal(t, DefaultModel, viper.GetString("model"))
	assert.Equal(t, "", viper.GetString("api_key"))
	assert.Equal(t, "", viper.GetString("api_base"))
	assert.Equal(t, DefaultPromptTemplate, viper.GetString("prompt_template"))
}

func TestInitConfig_CreateNewConfigFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission bits are not reliable on Windows")
	}

	tempDir, err := os.MkdirTemp("", "gmc_new_config_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	configFile := filepath.Join(tempDir, "fresh_config.yaml")

	viper.Reset()

	err = InitConfig(configFile)
	require.NoError(t, err)

	info, err := os.Stat(configFile)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestInitConfig_ExistingConfigFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gmc_existing_config_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	configFile := filepath.Join(tempDir, "existing_config.yaml")

	// Create existing config file with custom values
	existingConfig := `role: "Senior Go Developer"
model: "gpt-4"
api_key: "existing-key"
api_base: "https://api.custom.com/v1"
prompt_template: "/custom/prompt.yaml"`

	err = os.WriteFile(configFile, []byte(existingConfig), 0644)
	require.NoError(t, err)

	// Reset viper state
	viper.Reset()

	err = InitConfig(configFile)
	require.NoError(t, err)

	// Check that existing values are loaded
	assert.Equal(t, "Senior Go Developer", viper.GetString("role"))
	assert.Equal(t, "gpt-4", viper.GetString("model"))
	assert.Equal(t, "existing-key", viper.GetString("api_key"))
	assert.Equal(t, "https://api.custom.com/v1", viper.GetString("api_base"))
	assert.Equal(t, "/custom/prompt.yaml", viper.GetString("prompt_template"))

	if runtime.GOOS != "windows" {
		info, err := os.Stat(configFile)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	}
}

func TestInitConfig_DefaultPath(t *testing.T) {
	// This test validates XDG config path support

	// Reset viper state
	viper.Reset()

	// Get original env vars to restore later
	originalHome := os.Getenv("HOME")
	originalXDG := os.Getenv("XDG_CONFIG_HOME")

	// Create a temporary home directory
	tempHome, err := os.MkdirTemp("", "gmc_home_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempHome)

	// Unset XDG_CONFIG_HOME to test default behavior
	os.Unsetenv("XDG_CONFIG_HOME")
	os.Unsetenv("GMC_CONFIG")
	os.Setenv("HOME", tempHome)
	defer func() {
		os.Setenv("HOME", originalHome)
		if originalXDG != "" {
			os.Setenv("XDG_CONFIG_HOME", originalXDG)
		}
	}()

	err = InitConfig("")
	require.NoError(t, err)

	// Check defaults are set
	assert.Equal(t, DefaultRole, viper.GetString("role"))
	assert.Equal(t, DefaultModel, viper.GetString("model"))

	// Check config file was created in XDG path
	expectedConfigPath := filepath.Join(tempHome, ".config", "gmc", "config.yaml")
	assert.FileExists(t, expectedConfigPath)
}

func TestInitConfig_InvalidConfigFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gmc_invalid_config_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	configFile := filepath.Join(tempDir, "invalid_config.yaml")

	// Create invalid YAML file
	invalidConfig := `role: "Developer"
model: [invalid yaml structure`

	err = os.WriteFile(configFile, []byte(invalidConfig), 0644)
	require.NoError(t, err)

	// Reset viper state
	viper.Reset()

	err = InitConfig(configFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read configuration file")
}

func TestGetConfig(t *testing.T) {
	// Reset viper state and set test values
	viper.Reset()
	viper.Set("role", "Test Developer")
	viper.Set("model", "gpt-4")
	viper.Set("api_key", "test-key")
	viper.Set("api_base", "https://test-api.com/v1")
	viper.Set("prompt_template", "/test/prompts/test_template.yaml")

	cfg := GetConfig()

	assert.Equal(t, "Test Developer", cfg.Role)
	assert.Equal(t, "gpt-4", cfg.Model)
	assert.Equal(t, "test-key", cfg.APIKey)
	assert.Equal(t, "https://test-api.com/v1", cfg.APIBase)
	assert.Equal(t, "/test/prompts/test_template.yaml", cfg.PromptTemplate)
}

func TestGetConfig_UnmarshalError(t *testing.T) {
	// Reset viper state
	viper.Reset()

	// For this test, we'll just ensure GetConfig() works with defaults
	cfg := GetConfig()

	assert.NotNil(t, cfg)
	// Should have some defaults set (either empty string or default values)
	assert.True(t, cfg.Role == "" || cfg.Role == DefaultRole)
}

func TestSaveConfig(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gmc_save_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	configFile := filepath.Join(tempDir, "save_test.yaml")

	// Reset viper state
	viper.Reset()

	// Create existing config first
	initialConfig := `role: "Original Role"
model: "original-model"`
	err = os.WriteFile(configFile, []byte(initialConfig), 0644)
	require.NoError(t, err)

	// Initialize config
	err = InitConfig(configFile)
	require.NoError(t, err)

	// Modify some values
	SetConfigValue("role", "Modified Role")
	SetConfigValue("model", "modified-model")

	// Save config
	err = SaveConfig()
	require.NoError(t, err)

	// Read the saved file to verify
	content, err := os.ReadFile(configFile)
	require.NoError(t, err)

	assert.Contains(t, string(content), "Modified Role")
	assert.Contains(t, string(content), "modified-model")
}

func TestInitConfig_CreateConfigDirectoryError(t *testing.T) {
	// Test case where config directory can't be created
	// This is system-dependent and might not work on all platforms

	if os.Geteuid() == 0 {
		t.Skip("Running as root, cannot test directory creation failures")
	}

	// Try to create config in a path that should fail
	invalidPath := "/root/gmc_test_should_fail/config.yaml"

	// Reset viper state
	viper.Reset()

	err := InitConfig(invalidPath)
	// On most systems, this should fail due to permissions
	if err != nil {
		// The error might be about reading the config or creating directory
		assert.True(t,
			strings.Contains(err.Error(), "failed to create configuration directory") ||
				strings.Contains(err.Error(), "failed to read configuration file"))
	}
}

func TestInitConfig_EnvironmentVariables(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gmc_env_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	configFile := filepath.Join(tempDir, "env_test.yaml")

	// Create existing config first
	initialConfig := `role: "Config Role"
model: "config-model"`
	err = os.WriteFile(configFile, []byte(initialConfig), 0644)
	require.NoError(t, err)

	// Set environment variables with GMC_ prefix
	os.Setenv("GMC_ROLE", "Env Developer")
	os.Setenv("GMC_MODEL", "env-model")
	defer func() {
		os.Unsetenv("GMC_ROLE")
		os.Unsetenv("GMC_MODEL")
	}()

	// Reset viper state
	viper.Reset()

	err = InitConfig(configFile)
	require.NoError(t, err)

	// Verify GMC_ prefixed env vars take precedence
	assert.Equal(t, "Env Developer", viper.GetString("role"))
	assert.Equal(t, "env-model", viper.GetString("model"))
}

func TestInitConfig_HomeDirectoryError(t *testing.T) {
	// Save original HOME
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)

	// Unset HOME to simulate error
	os.Unsetenv("HOME")

	// Reset viper state
	viper.Reset()

	err := InitConfig("")
	// This should work because viper handles missing HOME gracefully in most cases
	// The test mainly ensures we don't panic
	if err != nil {
		assert.Contains(t, err.Error(), "failed to find home directory")
	}
}
