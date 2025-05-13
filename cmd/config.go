package cmd

import (
	"fmt"
	"github.com/samzong/gmc/internal/config"
	"github.com/samzong/gmc/internal/formatter"
	"github.com/spf13/cobra"
)

var (
	configCmd = &cobra.Command{
		Use:   "config",
		Short: "Manage gmc configuration",
		Long:  `Manage gmc configuration, including setting roles and LLM models, etc.`,
	}

	configSetCmd = &cobra.Command{
		Use:   "set",
		Short: "Set configuration item",
		Run: func(cmd *cobra.Command, args []string) {
		},
	}

	configSetRoleCmd = &cobra.Command{
		Use:   "role [Role Name]",
		Short: "Set Current Role",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			role := args[0]
			if !config.IsValidRole(role) {
				return fmt.Errorf("invalid role: %s", role)
			}

			config.SetConfigValue("role", role)

			if err := config.SaveConfig(); err != nil {
				return fmt.Errorf("Failed to save configuration: %w", err)
			}

			fmt.Printf("The role has been set to: %s\n", role)
			return nil
		},
	}

	configSetModelCmd = &cobra.Command{
		Use:   "model [Model Name]",
		Short: "Set up the LLM model",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			model := args[0]
			if !config.IsValidModel(model) {
				return fmt.Errorf("invalid model: %s", model)
			}

			config.SetConfigValue("model", model)

			if err := config.SaveConfig(); err != nil {
				return fmt.Errorf("Failed to save configuration: %w", err)
			}

			fmt.Printf("The model has been set to: %s\n", model)
			return nil
		},
	}

	configSetAPIKeyCmd = &cobra.Command{
		Use:   "apikey [API Key]",
		Short: "Set OpenAI API Key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			apiKey := args[0]

			config.SetConfigValue("api_key", apiKey)

			if err := config.SaveConfig(); err != nil {
				return fmt.Errorf("Failed to save configuration: %w", err)
			}

			fmt.Println("The API key has been set")
			return nil
		},
	}

	configSetAPIBaseCmd = &cobra.Command{
		Use:   "apibase [API Base URL]",
		Short: "Set OpenAI API Base URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			apiBase := args[0]

			config.SetConfigValue("api_base", apiBase)

			if err := config.SaveConfig(); err != nil {
				return fmt.Errorf("Failed to save configuration: %w", err)
			}

			fmt.Println("The API base URL has been set to:", apiBase)
			fmt.Println("Note: This setting is used for proxy OpenAI API, leave it empty if you don't need a proxy")
			return nil
		},
	}

	configSetPromptTemplateCmd = &cobra.Command{
		Use:   "prompt_template [Template Name or Path]",
		Short: "Set Prompt Template",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			templateName := args[0]

			_, err := formatter.GetPromptTemplate(templateName)
			if err != nil {
				return fmt.Errorf("invalid prompt template: %s, error: %w", templateName, err)
			}

			config.SetConfigValue("prompt_template", templateName)

			if err := config.SaveConfig(); err != nil {
				return fmt.Errorf("Failed to save configuration: %w", err)
			}

			fmt.Printf("The prompt template has been set to: %s\n", templateName)
			return nil
		},
	}

	configSetCustomPromptsDirCmd = &cobra.Command{
		Use:   "custom_prompts_dir [Directory Path]",
		Short: "Set Custom Prompt Template Directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := args[0]

			config.SetConfigValue("custom_prompts_dir", dir)

			if err := config.SaveConfig(); err != nil {
				return fmt.Errorf("Failed to save configuration: %w", err)
			}

			fmt.Printf("The custom prompt template directory has been set to: %s\n", dir)
			return nil
		},
	}

	configGetCmd = &cobra.Command{
		Use:   "get",
		Short: "Get Current Configuration",
		Run: func(cmd *cobra.Command, args []string) {
			cfg := config.GetConfig()
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
			fmt.Printf("Custom Prompt Template Directory: %s\n", cfg.CustomPromptsDir)
		},
	}

	configListTemplatesCmd = &cobra.Command{
		Use:   "list_templates",
		Short: "List All Available Prompt Templates",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Built-in Templates:")
			for name := range formatter.GetBuiltinTemplates() {
				fmt.Printf("- %s\n", name)
			}

			cfg := config.GetConfig()
			if cfg.CustomPromptsDir != "" {
				fmt.Printf("\nCustom Template Directory (%s):\n", cfg.CustomPromptsDir)
				templates, err := formatter.ListCustomTemplates(cfg.CustomPromptsDir)
				if err != nil {
					fmt.Printf("Failed to read custom templates: %v\n", err)
				} else {
					if len(templates) == 0 {
						fmt.Println("No custom templates found")
					} else {
						for _, tpl := range templates {
							fmt.Printf("- %s\n", tpl)
						}
					}
				}
			}
		},
	}
)

func init() {
	configSetCmd.AddCommand(configSetRoleCmd)
	configSetCmd.AddCommand(configSetModelCmd)
	configSetCmd.AddCommand(configSetAPIKeyCmd)
	configSetCmd.AddCommand(configSetAPIBaseCmd)
	configSetCmd.AddCommand(configSetPromptTemplateCmd)
	configSetCmd.AddCommand(configSetCustomPromptsDirCmd)

	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configListTemplatesCmd)
}
