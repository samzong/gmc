package config

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

func TestExplicitAPIKeyStorage(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		t.Run(map[bool]string{false: "available", true: "unavailable"}[unavailable], func(t *testing.T) {
			const original = "API_KEY: first-fixture\napi_key: second-fixture\nmodel: fixture-model\n"
			path := arrangeRepoConfig(t, original, "")
			require.NoError(t, InitConfig(path))
			account, err := credentialAccount()
			require.NoError(t, err)
			_, err = keyring.Get(credentialService, account)
			require.ErrorIs(t, err, keyring.ErrNotFound)
			if unavailable {
				keyring.MockInitWithError(errors.New("storage unavailable"))
			}
			err = SetConfigValue("api_key", "selected-fixture")
			if unavailable {
				require.Error(t, err)
				content, readErr := os.ReadFile(path)
				require.NoError(t, readErr)
				assert.Equal(t, original, string(content))
				return
			}
			require.NoError(t, err)
			require.NoError(t, SaveConfig())
			stored, err := keyring.Get(credentialService, account)
			require.NoError(t, err)
			assert.Equal(t, "selected-fixture", stored)
			content, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.NotContains(t, strings.ToLower(string(content)), "api_key")
		})
	}
}

func TestSaveConfigPreservesOnlyFileAPIKeys(t *testing.T) {
	const original = "API_KEY: first-fixture\napi_key: 'second-fixture'\nmodel: fixture-model\n"
	path := arrangeRepoConfig(t, original, "")
	require.NoError(t, InitConfig(path))
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(content))
	account, err := credentialAccount()
	require.NoError(t, err)
	require.NoError(t, keyring.Set(credentialService, account, "stored-fixture"))
	t.Setenv("GMC_API_KEY", "environment-fixture")
	viper.Set("api_key", "override-fixture")
	require.NoError(t, SetConfigValue("model", "updated-model"))
	require.NoError(t, SaveConfig())
	content, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(content), "API_KEY: first-fixture\n")
	assert.Contains(t, string(content), "api_key: 'second-fixture'\n")
	for _, excluded := range []string{"stored-fixture", "environment-fixture", "override-fixture"} {
		assert.NotContains(t, string(content), excluded)
	}
	require.NoError(t, os.Remove(path))
	require.NoError(t, SaveConfig())
	content, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, strings.ToLower(string(content)), "api_key")
}
