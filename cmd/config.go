package cmd

import (
	"fmt"
	"github.com/samzong/gma/internal/config"
	"github.com/spf13/cobra"
)

var (
	configCmd = &cobra.Command{
		Use:   "config",
		Short: "管理GMA配置",
		Long:  `管理GMA配置，包括设置角色和LLM模型等`,
	}

	configSetCmd = &cobra.Command{
		Use:   "set",
		Short: "设置配置项",
		Run: func(cmd *cobra.Command, args []string) {
			// 配置项设置逻辑在子命令中实现
		},
	}

	configSetRoleCmd = &cobra.Command{
		Use:   "role [角色名]",
		Short: "设置当前角色",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			role := args[0]
			if !config.IsValidRole(role) {
				return fmt.Errorf("无效的角色: %s", role)
			}
			
			cfg := config.GetConfig()
			cfg.Role = role
			if err := config.SaveConfig(); err != nil {
				return fmt.Errorf("保存配置失败: %w", err)
			}
			
			fmt.Printf("已设置角色为: %s\n", role)
			fmt.Println("提示: 您可以使用任何角色名称，建议角色有:")
			for _, r := range config.GetSuggestedRoles() {
				fmt.Printf("- %s\n", r)
			}
			return nil
		},
	}

	configSetModelCmd = &cobra.Command{
		Use:   "model [模型名]",
		Short: "设置LLM模型",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			model := args[0]
			if !config.IsValidModel(model) {
				return fmt.Errorf("无效的模型: %s", model)
			}
			
			cfg := config.GetConfig()
			cfg.Model = model
			if err := config.SaveConfig(); err != nil {
				return fmt.Errorf("保存配置失败: %w", err)
			}
			
			fmt.Printf("已设置模型为: %s\n", model)
			fmt.Println("提示: 您可以使用任何模型名称，建议模型有:")
			for _, m := range config.GetSuggestedModels() {
				fmt.Printf("- %s\n", m)
			}
			return nil
		},
	}

	configSetAPIKeyCmd = &cobra.Command{
		Use:   "apikey [API密钥]",
		Short: "设置OpenAI API密钥",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			apiKey := args[0]
			
			cfg := config.GetConfig()
			cfg.APIKey = apiKey
			if err := config.SaveConfig(); err != nil {
				return fmt.Errorf("保存配置失败: %w", err)
			}
			
			fmt.Println("已设置API密钥")
			return nil
		},
	}

	configSetAPIBaseCmd = &cobra.Command{
		Use:   "apibase [API基础URL]",
		Short: "设置OpenAI API基础URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			apiBase := args[0]
			
			cfg := config.GetConfig()
			cfg.APIBase = apiBase
			if err := config.SaveConfig(); err != nil {
				return fmt.Errorf("保存配置失败: %w", err)
			}
			
			fmt.Println("已设置API基础URL为:", apiBase)
			fmt.Println("提示: 此设置用于代理OpenAI API，如不需要代理请留空")
			return nil
		},
	}

	configGetCmd = &cobra.Command{
		Use:   "get",
		Short: "获取当前配置",
		Run: func(cmd *cobra.Command, args []string) {
			cfg := config.GetConfig()
			fmt.Println("当前配置:")
			fmt.Printf("角色: %s\n", cfg.Role)
			fmt.Printf("模型: %s\n", cfg.Model)
			fmt.Println("API密钥: ********")
			if cfg.APIBase != "" {
				fmt.Printf("API基础URL: %s\n", cfg.APIBase)
			} else {
				fmt.Println("API基础URL: <未设置>")
			}
		},
	}
)

func init() {
	// 添加config子命令
	configSetCmd.AddCommand(configSetRoleCmd)
	configSetCmd.AddCommand(configSetModelCmd)
	configSetCmd.AddCommand(configSetAPIKeyCmd)
	configSetCmd.AddCommand(configSetAPIBaseCmd)
	
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
} 