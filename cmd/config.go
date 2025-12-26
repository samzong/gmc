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
		Run: func(_ *cobra.Command, _ []string) {
		},
	}

	configSetRoleCmd = &cobra.Command{
		Use:   "role [Role Name]",
		Short: "Set Current Role",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			role := args[0]
			if !config.IsValidRole(role) {
				return fmt.Errorf("invalid role: %s", role)
			}

			config.SetConfigValue("role", role)

			if err := config.SaveConfig(); err != nil {
				return fmt.Errorf("failed to save configuration: %w", err)
			}

			fmt.Fprintf(os.Stderr, "The role has been set to: %s\n", role)
			return nil
		},
	}

	configSetModelCmd = &cobra.Command{
		Use:   "model [Model Name]",
		Short: "Set up the LLM model",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			model := args[0]
			if !config.IsValidModel(model) {
				return fmt.Errorf("invalid model: %s", model)
			}

			config.SetConfigValue("model", model)

			if err := config.SaveConfig(); err != nil {
				return fmt.Errorf("failed to save configuration: %w", err)
			}

			fmt.Fprintf(os.Stderr, "The model has been set to: %s\n", model)
			return nil
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
			if !isatty.IsTerminal(os.Stdin.Fd()) && !isatty.IsCygwinTerminal(os.Stdin.Fd()) {
				return errors.New("this command requires an interactive terminal")
			}

			fmt.Fprint(os.Stderr, "Enter API Key: ")
			keyBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Fprintln(os.Stderr) // newline after hidden input
			if err != nil {
				return fmt.Errorf("failed to read API key: %w", err)
			}

			apiKey := strings.TrimSpace(string(keyBytes))
			if apiKey == "" {
				return errors.New("API key cannot be empty")
			}

			config.SetConfigValue("api_key", apiKey)

			if err := config.SaveConfig(); err != nil {
				return fmt.Errorf("failed to save configuration: %w", err)
			}

			fmt.Fprintln(os.Stderr, "The API key has been set")
			return nil
		},
	}

	configSetAPIBaseCmd = &cobra.Command{
		Use:   "apibase [API Base URL]",
		Short: "Set OpenAI API Base URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			apiBase := args[0]

			config.SetConfigValue("api_base", apiBase)

			if err := config.SaveConfig(); err != nil {
				return fmt.Errorf("failed to save configuration: %w", err)
			}

			fmt.Fprintln(os.Stderr, "The API base URL has been set to:", apiBase)
			fmt.Fprintln(os.Stderr, "Note: This setting is used for proxy OpenAI API, leave it empty if you don't need a proxy")
			return nil
		},
	}

	configSetPromptTemplateCmd = &cobra.Command{
		Use:   "prompt_template [Template Path]",
		Short: "Set Prompt Template",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			templateName := args[0]

			_, err := formatter.GetPromptTemplate(templateName)
			if err != nil {
				return fmt.Errorf("invalid prompt template: %s, error: %w", templateName, err)
			}

			config.SetConfigValue("prompt_template", templateName)

			if err := config.SaveConfig(); err != nil {
				return fmt.Errorf("failed to save configuration: %w", err)
			}

			fmt.Fprintf(os.Stderr, "The prompt template has been set to: %s\n", templateName)
			return nil
		},
	}

	configSetEnableEmojiCmd = &cobra.Command{
		Use:   "enable_emoji [true|false]",
		Short: "Enable or disable emoji in commit messages",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
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

			if err := config.SaveConfig(); err != nil {
				return fmt.Errorf("failed to save configuration: %w", err)
			}

			if enableEmoji {
				fmt.Fprintln(os.Stderr, "Emoji support has been enabled")
			} else {
				fmt.Fprintln(os.Stderr, "Emoji support has been disabled")
			}
			return nil
		},
	}

	configGetCmd = &cobra.Command{
		Use:   "get",
		Short: "Get Current Configuration",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg := config.GetConfig()

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
				encoder := json.NewEncoder(os.Stdout)
				encoder.SetIndent("", "  ")
				return encoder.Encode(output)
			}

			// Human-readable output to stdout (main data output)
			fmt.Println("Current Configuration:")
			fmt.Printf("Role: %s\n", cfg.Role)
			fmt.Printf("Model: %s\n", cfg.Model)
			fmt.Println("API Key: ********")
			if cfg.APIBase != "" {
				fmt.Printf("API Base URL: %s\n", cfg.APIBase)
			} else {
				fmt.Println("API Base URL: <Not Set>")
			}
			fmt.Printf("Prompt Template: %s\n", cfg.PromptTemplate)
			fmt.Printf("Enable Emoji: %v\n", cfg.EnableEmoji)
			return nil
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
