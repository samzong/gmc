package cmd

import (
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
		Short: "Set the current role",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runConfigSetRole(args)
		},
	}

	configSetModelCmd = &cobra.Command{
		Use:   "model [Model Name]",
		Short: "Set the LLM model",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runConfigSetModel(args)
		},
	}

	configSetAPIKeyCmd = &cobra.Command{
		Use:   "apikey",
		Short: "Set the OpenAI API key (hidden input)",
		Long:  `Read the OpenAI API key from a hidden interactive prompt. A terminal is required.`,
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runConfigSetAPIKey()
		},
	}

	configSetAPIBaseCmd = &cobra.Command{
		Use:   "apibase [API Base URL]",
		Short: "Set the OpenAI API base URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runConfigSetAPIBase(args)
		},
	}

	configSetPromptTemplateCmd = &cobra.Command{
		Use:   "prompt_template [Template Path]",
		Short: "Set the prompt template path",
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
		Short: "Show current configuration",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runConfigGet()
		},
	}
)

type configJSONOutput struct {
	Role           string `json:"role"`
	Model          string `json:"model"`
	APIKeySet      bool   `json:"api_key_set"`
	APIBase        string `json:"api_base"`
	PromptTemplate string `json:"prompt_template"`
	EnableEmoji    bool   `json:"enable_emoji"`
}

func setConfigValue(key string, value any, message string) error {
	config.SetConfigValue(key, value)
	if err := config.SaveConfig(); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}
	fmt.Fprintln(outWriter(), message)
	return nil
}

func runConfigSetRole(args []string) error {
	role := args[0]
	if !config.IsValidRole(role) {
		return fmt.Errorf("invalid role: %s", role)
	}

	return setConfigValue("role", role, "The role has been set to: "+role)
}

func runConfigSetModel(args []string) error {
	model := args[0]
	if !config.IsValidModel(model) {
		return fmt.Errorf("invalid model: %s", model)
	}

	return setConfigValue("model", model, "The model has been set to: "+model)
}

func runConfigSetAPIKey() error {
	if !isatty.IsTerminal(os.Stdin.Fd()) && !isatty.IsCygwinTerminal(os.Stdin.Fd()) {
		return errors.New("this command requires an interactive terminal")
	}

	fmt.Fprint(errWriter(), "Enter API Key: ")
	keyBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(errWriter())
	if err != nil {
		return fmt.Errorf("failed to read API key: %w", err)
	}

	apiKey := strings.TrimSpace(string(keyBytes))
	if apiKey == "" {
		return errors.New("API key cannot be empty")
	}

	return setConfigValue("api_key", apiKey, "The API key has been set")
}

func runConfigSetAPIBase(args []string) error {
	return setConfigValue("api_base", args[0], "The API base URL has been set to: "+args[0]+
		"\nNote: This setting is used for proxy OpenAI API, leave it empty if you don't need a proxy")
}

func runConfigSetPromptTemplate(args []string) error {
	templateName := args[0]

	_, err := formatter.GetPromptTemplate(templateName)
	if err != nil {
		return fmt.Errorf("invalid prompt template: %s, error: %w", templateName, err)
	}

	return setConfigValue("prompt_template", templateName, "The prompt template has been set to: "+templateName)
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

	message := "Emoji support has been disabled"
	if enableEmoji {
		message = "Emoji support has been enabled"
	}
	return setConfigValue("enable_emoji", enableEmoji, message)
}

func runConfigGet() error {
	cfg, err := config.GetConfig()
	if err != nil {
		return err
	}

	if configOutputJSON || outputFormat() == "json" {
		output := configJSONOutput{
			Role:           cfg.Role,
			Model:          cfg.Model,
			APIKeySet:      cfg.APIKey != "",
			APIBase:        cfg.APIBase,
			PromptTemplate: cfg.PromptTemplate,
			EnableEmoji:    cfg.EnableEmoji,
		}
		return printJSON(outWriter(), output)
	}

	fmt.Fprintln(outWriter(), "Current Configuration:")
	fmt.Fprintf(outWriter(), "Role: %s\n", sanitizeForTerminal(cfg.Role))
	fmt.Fprintf(outWriter(), "Model: %s\n", sanitizeForTerminal(cfg.Model))
	fmt.Fprintln(outWriter(), "API Key: ********")
	if cfg.APIBase != "" {
		fmt.Fprintf(outWriter(), "API Base URL: %s\n", sanitizeForTerminal(cfg.APIBase))
	} else {
		fmt.Fprintln(outWriter(), "API Base URL: <Not Set>")
	}
	fmt.Fprintf(outWriter(), "Prompt Template: %s\n", sanitizeForTerminal(cfg.PromptTemplate))
	fmt.Fprintf(outWriter(), "Enable Emoji: %v\n", cfg.EnableEmoji)
	return nil
}

func init() {
	configSetCmd.AddCommand(configSetRoleCmd, configSetModelCmd, configSetAPIKeyCmd,
		configSetAPIBaseCmd, configSetPromptTemplateCmd, configSetEnableEmojiCmd)

	configGetCmd.Flags().BoolVar(&configOutputJSON, "json", false, "Output in JSON format (deprecated: use -o json)")
	_ = configGetCmd.Flags().MarkDeprecated("json", "use -o json instead")

	configCmd.AddCommand(configSetCmd, configGetCmd)
}
