package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

func TestPublicConfigFilePrecedence(t *testing.T) {
	for _, mode := range []string{"flag", "environment", "xdg", "legacy"} {
		t.Run(mode, func(t *testing.T) {
			xdg := arrangeRepoConfig(t, "model: xdg-model\n", "model: repo-model\n")
			legacy := filepath.Join(os.Getenv("HOME"), ".gmc.yaml")
			environment := filepath.Join(t.TempDir(), "environment.yaml")
			flag := filepath.Join(t.TempDir(), "flag.yaml")
			for path, model := range map[string]string{
				legacy: "legacy-model", environment: "environment-model", flag: "flag-model",
			} {
				require.NoError(t, os.WriteFile(path, []byte("model: "+model+"\n"), 0o600))
			}
			selected, wantModel, argument := xdg, "repo-model", ""
			switch mode {
			case "flag":
				t.Setenv("GMC_CONFIG", environment)
				selected, wantModel, argument = flag, "flag-model", flag
			case "environment":
				t.Setenv("GMC_CONFIG", environment)
				selected, wantModel = environment, "environment-model"
			case "legacy":
				require.NoError(t, os.Remove(xdg))
				selected = legacy
			}
			require.NoError(t, InitConfig(argument))
			assert.Equal(t, selected, FilePath())
			assert.Equal(t, wantModel, MustGetSettings().Model)
			assert.Equal(t, mode == "flag" || mode == "environment", RepoConfig().Skipped)
			require.NoError(t, os.Remove(filepath.Join(mustWorkingDirectory(t), ".gmc.yaml")))
			viper.Reset()
			require.NoError(t, InitConfig(argument))
			assert.Equal(t, mode+"-model", MustGetSettings().Model)
		})
	}
}

func mustWorkingDirectory(t *testing.T) string {
	t.Helper()
	path, err := os.Getwd()
	require.NoError(t, err)
	return path
}

func TestPublicAPIKeyPrecedence(t *testing.T) {
	for _, mode := range []string{"environment", "stored", "file", "unavailable", "empty"} {
		t.Run(mode, func(t *testing.T) {
			content := "api_key: file-fixture\n"
			if mode == "empty" {
				content = ""
			}
			path := arrangeRepoConfig(t, content, "api_key: repo-fixture\n")
			require.NoError(t, InitConfig(path))
			var warnings bytes.Buffer
			previous := credentialWarningWriter
			SetCredentialWarningWriter(&warnings)
			t.Cleanup(func() { SetCredentialWarningWriter(previous) })
			account, err := credentialAccount()
			require.NoError(t, err)
			want := "file-fixture"
			switch mode {
			case "environment", "stored":
				require.NoError(t, keyring.Set(credentialService, account, "stored-fixture"))
				want = "stored-fixture"
				if mode == "environment" {
					t.Setenv("GMC_API_KEY", "environment-fixture")
					want = "environment-fixture"
				}
			case "unavailable":
				keyring.MockInitWithError(errors.New("unavailable"))
			case "empty":
				want = ""
			}
			MustGetSettings()
			assert.False(t, credentials.loaded)
			for range 2 {
				cfg, err := GetConfig()
				require.NoError(t, err)
				assert.Equal(t, want, cfg.APIKey)
			}
			if mode == "environment" {
				assert.False(t, credentials.loaded)
			}
			if mode == "file" || mode == "unavailable" {
				assert.Equal(t, 1, bytes.Count(warnings.Bytes(), []byte("gmc:")))
				assert.Contains(t, warnings.String(), "plaintext")
			} else {
				assert.Empty(t, warnings.String())
			}
			for _, value := range []string{"file-fixture", "stored-fixture", "environment-fixture", "repo-fixture"} {
				assert.NotContains(t, warnings.String(), value)
			}
		})
	}
}
