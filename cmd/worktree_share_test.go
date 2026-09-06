package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samzong/gmc/internal/worktree"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscoverRejectsExplicitDryRunWithAutoBeforeLoadingConfig(t *testing.T) {
	previousAuto := discoverAuto
	t.Cleanup(func() { discoverAuto = previousAuto })
	discoverAuto = true
	for _, value := range []string{"true", "false"} {
		t.Run(value, func(t *testing.T) {
			command := &cobra.Command{Use: "discover"}
			command.Flags().Bool("dry-run", true, "")
			require.NoError(t, command.Flags().Set("dry-run", value))
			err := discoverSharedResources(command, nil)
			require.Error(t, err)
		})
	}
}

func TestDiscoverPreviewDoesNotConfigureOrRunHooks(t *testing.T) {
	repo := initCmdTestRepo(t)
	t.Chdir(repo)
	globalPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(globalPath, []byte("worktree: {}\n"), 0o600))
	configPath := filepath.Join(repo, ".git", "gmc-share.yml")
	originalConfig := []byte("hooks:\n  - id: install\n    cmd: touch hook-ran\n")
	require.NoError(t, os.WriteFile(configPath, originalConfig, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".env"), []byte("EXAMPLE=1\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "apps", "web", "node_modules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "apps", "web", "package.json"), []byte("{}"), 0o600))

	previousAuto, previousOutput := discoverAuto, outputFlag.value
	t.Cleanup(func() {
		discoverAuto = previousAuto
		outputFlag.value = previousOutput
	})
	discoverAuto = false
	outputFlag.value = "json"
	command := &cobra.Command{Use: "discover"}
	command.Flags().Bool("dry-run", true, "")
	require.NoError(t, command.Flags().Set("dry-run", "false"))
	var output bytes.Buffer
	command.SetOut(&output)
	client := worktree.NewClient(worktree.Options{GlobalConfigPath: globalPath})
	require.NoError(t, discoverSharedResources(command, client))

	var preview struct {
		Resources []worktree.DiscoverResult `json:"resources"`
		Hooks     []HookJSON                `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal(output.Bytes(), &preview))
	require.Len(t, preview.Hooks, 1)
	assert.Equal(t, "install", preview.Hooks[0].ID)
	var foundNested bool
	for _, resource := range preview.Resources {
		if resource.Path == "apps/web/node_modules" {
			foundNested = true
			assert.NotEqual(t, "candidate", resource.Status)
		}
	}
	assert.True(t, foundNested)
	output.Reset()
	outputFlag.value = "text"
	require.NoError(t, discoverSharedResources(command, client))
	text := output.String()
	directoryPosition := strings.Index(text, "apps/web/node_modules")
	filePosition := strings.Index(text, ".env")
	require.GreaterOrEqual(t, directoryPosition, 0)
	require.Greater(t, filePosition, directoryPosition)
	assert.Contains(t, text, "Node.js (npm): 1")
	assert.NotContains(t, text, "package.json")
	assert.NotContains(t, text, "touch hook-ran")
	actualConfig, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, originalConfig, actualConfig)
	_, err = os.Stat(filepath.Join(repo, "hook-ran"))
	assert.True(t, os.IsNotExist(err))
}

func TestHookListIndicesMatchEffectiveRemoval(t *testing.T) {
	repo := initCmdTestRepo(t)
	t.Chdir(repo)
	globalPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(globalPath,
		[]byte("worktree:\n  hooks:\n    - id: install\n      cmd: echo install\n"+
			"    - id: setup\n      cmd: echo setup\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".git", "gmc-share.yml"),
		[]byte("hooks:\n  - id: install\n    disabled: true\n"), 0o600))
	client := worktree.NewClient(worktree.Options{GlobalConfigPath: globalPath})
	previousOutput := outputFlag.value
	t.Cleanup(func() { outputFlag.value = previousOutput })
	outputFlag.value = "json"
	command := &cobra.Command{Use: "list"}
	command.Flags().Bool("global", false, "")
	var output bytes.Buffer
	command.SetOut(&output)
	require.NoError(t, runHookListCommand(command, client))
	var hooks []HookJSON
	require.NoError(t, json.Unmarshal(output.Bytes(), &hooks))
	require.Len(t, hooks, 2)
	assert.True(t, hooks[0].Disabled)
	assert.Equal(t, "setup", hooks[1].ID)
	_, err := client.RemoveHook(hooks[1].Index - 1)
	require.NoError(t, err)
	cfg, err := client.LoadEffectiveSharedConfig()
	require.NoError(t, err)
	require.Len(t, cfg.Hooks, 2)
	assert.True(t, cfg.Hooks[1].Disabled)
	global, _, err := client.LoadGlobalSharedConfig()
	require.NoError(t, err)
	assert.False(t, global.Hooks[1].Disabled)

	output.Reset()
	require.NoError(t, command.Flags().Set("global", "true"))
	require.NoError(t, runHookListCommand(command, client))
	require.NoError(t, json.Unmarshal(output.Bytes(), &hooks))
	require.Len(t, hooks, 2)
	assert.Equal(t, "global", hooks[0].Origin)
	assert.False(t, hooks[0].Disabled)
	assert.False(t, hooks[1].Disabled)
}
