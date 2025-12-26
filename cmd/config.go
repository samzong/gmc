package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/samzong/gmc/internal/config"
	"github.com/samzong/gmc/internal/formatter"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	configOutputJSON bool

	configCmd = &cobra.Command{
		Use:   "config",
		Short: "Manage gmc configuration",
		Long:  `Manage gmc configuration, including setting roles and LLM models, etc.`,
	}

	configSetCmd = &cobra.Command{
		Use:   "set",
		Short: "Set configuration item",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	configSetRoleCmd = &cobra.Command{
		Use:   "role [Role Name]",
		Short: "Set Current Role",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runConfigSetRole(args)
		},
	}

	configSetModelCmd = &cobra.Command{
		Use:   "model [Model Name]",
		Short: "Set up the LLM model",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runConfigSetModel(args)
		},
	}

	configSetAPIKeyCmd = &cobra.Command{
		Use:   "apikey",
		Short: "Set OpenAI API Key (interactive, hidden input)",
		Long: `Set OpenAI API Key securely with hidden input.

For security, the key must be entered interactively (input is hidden).
This command requires a terminal.

Usage:
  gmc config set apikey`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runConfigSetAPIKey()
		},
	}

	configSetAPIBaseCmd = &cobra.Command{
		Use:   "apibase [API Base URL]",
		Short: "Set OpenAI API Base URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runConfigSetAPIBase(args)
		},
	}

	configSetPromptTemplateCmd = &cobra.Command{
		Use:   "prompt_template [Template Path]",
		Short: "Set Prompt Template",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runConfigSetPromptTemplate(args)
		},
	}

	configSetEnableEmojiCmd = &cobra.Command{
		Use:   "enable_emoji [true|false]",
		Short: "Enable or disable emoji in commit messages",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runConfigSetEnableEmoji(args)
		},
	}

	configGetCmd = &cobra.Command{
		Use:   "get",
		Short: "Get Current Configuration",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runConfigGet()
		},
	}
)

// configJSONOutput is the JSON structure for config get --json
type configJSONOutput struct {
	Role           string `json:"role"`
	Model          string `json:"model"`
	APIKeySet      bool   `json:"api_key_set"`
	APIBase        string `json:"api_base"`
	PromptTemplate string `json:"prompt_template"`
	EnableEmoji    bool   `json:"enable_emoji"`
}

func saveConfig() error {
	if err := config.SaveConfig(); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}
	return nil
}

func runConfigSetRole(args []string) error {
	role := args[0]
	if !config.IsValidRole(role) {
		return fmt.Errorf("invalid role: %s", role)
	}

	config.SetConfigValue("role", role)

	if err := saveConfig(); err != nil {
		return err
	}

	fmt.Fprintf(outWriter(), "The role has been set to: %s\n", role)
	return nil
}

func runConfigSetModel(args []string) error {
	model := args[0]
	if !config.IsValidModel(model) {
		return fmt.Errorf("invalid model: %s", model)
	}

	config.SetConfigValue("model", model)

	if err := saveConfig(); err != nil {
		return err
	}

	fmt.Fprintf(outWriter(), "The model has been set to: %s\n", model)
	return nil
}

func runConfigSetAPIKey() error {
	if !isatty.IsTerminal(os.Stdin.Fd()) && !isatty.IsCygwinTerminal(os.Stdin.Fd()) {
		return errors.New("this command requires an interactive terminal")
	}

	fmt.Fprint(errWriter(), "Enter API Key: ")
	keyBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(errWriter()) // newline after hidden input
	if err != nil {
		return fmt.Errorf("failed to read API key: %w", err)
	}

	apiKey := strings.TrimSpace(string(keyBytes))
	if apiKey == "" {
		return errors.New("API key cannot be empty")
	}

	config.SetConfigValue("api_key", apiKey)

	if err := saveConfig(); err != nil {
		return err
	}

	fmt.Fprintln(outWriter(), "The API key has been set")
	return nil
}

func runConfigSetAPIBase(args []string) error {
	apiBase := args[0]

	config.SetConfigValue("api_base", apiBase)

	if err := saveConfig(); err != nil {
		return err
	}

	fmt.Fprintln(outWriter(), "The API base URL has been set to:", apiBase)
	fmt.Fprintln(outWriter(), "Note: This setting is used for proxy OpenAI API, leave it empty if you don't need a proxy")
	return nil
}

func runConfigSetPromptTemplate(args []string) error {
	templateName := args[0]

	_, err := formatter.GetPromptTemplate(templateName)
	if err != nil {
		return fmt.Errorf("invalid prompt template: %s, error: %w", templateName, err)
	}

	config.SetConfigValue("prompt_template", templateName)

	if err := saveConfig(); err != nil {
		return err
	}

	fmt.Fprintf(outWriter(), "The prompt template has been set to: %s\n", templateName)
	return nil
}

func runConfigSetEnableEmoji(args []string) error {
	value := args[0]
	var enableEmoji bool
	switch value {
	case "true":
		enableEmoji = true
	case "false":
		enableEmoji = false
	default:
		return fmt.Errorf("invalid value: %s (must be 'true' or 'false')", value)
	}

	config.SetConfigValue("enable_emoji", enableEmoji)

	if err := saveConfig(); err != nil {
		return err
	}

	if enableEmoji {
		fmt.Fprintln(outWriter(), "Emoji support has been enabled")
	} else {
		fmt.Fprintln(outWriter(), "Emoji support has been disabled")
	}
	return nil
}

func runConfigGet() error {
	cfg, err := config.GetConfig()
	if err != nil {
		return err
	}

	if configOutputJSON {
		// JSON output to stdout for machine consumption
		output := configJSONOutput{
			Role:           cfg.Role,
			Model:          cfg.Model,
			APIKeySet:      cfg.APIKey != "",
			APIBase:        cfg.APIBase,
			PromptTemplate: cfg.PromptTemplate,
			EnableEmoji:    cfg.EnableEmoji,
		}
		encoder := json.NewEncoder(outWriter())
		encoder.SetIndent("", "  ")
		return encoder.Encode(output)
	}

	// Human-readable output to stdout (main data output)
	fmt.Fprintln(outWriter(), "Current Configuration:")
	fmt.Fprintf(outWriter(), "Role: %s\n", cfg.Role)
	fmt.Fprintf(outWriter(), "Model: %s\n", cfg.Model)
	fmt.Fprintln(outWriter(), "API Key: ********")
	if cfg.APIBase != "" {
		fmt.Fprintf(outWriter(), "API Base URL: %s\n", cfg.APIBase)
	} else {
		fmt.Fprintln(outWriter(), "API Base URL: <Not Set>")
	}
	fmt.Fprintf(outWriter(), "Prompt Template: %s\n", cfg.PromptTemplate)
	fmt.Fprintf(outWriter(), "Enable Emoji: %v\n", cfg.EnableEmoji)
	return nil
}

func init() {
	configSetCmd.AddCommand(configSetRoleCmd)
	configSetCmd.AddCommand(configSetModelCmd)
	configSetCmd.AddCommand(configSetAPIKeyCmd)
	configSetCmd.AddCommand(configSetAPIBaseCmd)
	configSetCmd.AddCommand(configSetPromptTemplateCmd)
	configSetCmd.AddCommand(configSetEnableEmojiCmd)

	configGetCmd.Flags().BoolVar(&configOutputJSON, "json", false, "Output configuration in JSON format")

	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
}
