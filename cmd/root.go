package cmd

import (
	"github.com/samzong/gma/internal/git"
	"github.com/samzong/gma/internal/llm"
	"github.com/samzong/gma/internal/config"
	"github.com/samzong/gma/internal/formatter"
	"github.com/spf13/cobra"
	"fmt"
	"os"
)

var (
	cfgFile   string
	noVerify  bool
	dryRun    bool
	addAll    bool // 添加变量控制是否执行git add .
	issueNum  string // issue编号
	rootCmd   = &cobra.Command{
		Use:   "gma",
		Short: "GMA - Git Message Assistant",
		Long: `GMA 是一个加速Git提交效率的CLI工具，通过LLM智能生成高质量的commit message。
使用GMA可以一键完成git add和commit操作，减少开发者在提交代码时的心智负担。`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleErrors(generateAndCommit())
		},
		// 阻止Cobra在遇到错误时自动打印出错误信息和使用帮助
		SilenceErrors: true,
		SilenceUsage:  true,
	}
)

// Execute 执行根命令
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "配置文件路径 (默认为 $HOME/.gma.yaml)")
	rootCmd.Flags().BoolVar(&noVerify, "no-verify", false, "跳过 pre-commit 钩子")
	rootCmd.Flags().BoolVar(&dryRun, "dry-run", false, "仅生成消息，不实际提交")
	rootCmd.Flags().BoolVarP(&addAll, "all", "a", false, "自动将所有变更添加到暂存区后再提交")
	rootCmd.Flags().StringVar(&issueNum, "issue", "", "关联的issue编号(可选)")

	// 添加子命令
	rootCmd.AddCommand(configCmd)
}

func initConfig() {
	config.InitConfig(cfgFile)
}

// 处理错误，对特定错误使用更友好的提示
func handleErrors(err error) error {
	if err != nil {
		// 对特定错误使用自定义处理
		if err.Error() == "没有检测到暂存区的文件变更" {
			fmt.Println("没有检测到暂存区的文件变更")
			if !addAll {
				fmt.Println("提示: 可以使用 -a 或 --all 参数自动添加所有变更到暂存区")
			}
			return nil // 不算作错误退出
		} else if err.Error() == "没有检测到文件变更" {
			fmt.Println("没有检测到文件变更")
			return nil // 不算作错误退出
		}
		
		// 其他错误直接打印，不返回给Cobra处理
		fmt.Fprintln(os.Stderr, "错误:", err)
		return nil // 不算作错误退出，避免显示使用帮助
	}
	return nil
}

func generateAndCommit() error {
	// 如果指定了--all/-a参数，先执行git add .
	if addAll {
		if err := git.AddAll(); err != nil {
			return fmt.Errorf("git add 失败: %w", err)
		}
		fmt.Println("已将所有变更添加到暂存区")
	}

	// 1. 获取 git diff 信息（只获取暂存区的变更）
	diff, err := git.GetStagedDiff()
	if err != nil {
		return fmt.Errorf("获取Git差异失败: %w", err)
	}

	if diff == "" {
		return fmt.Errorf("没有检测到暂存区的文件变更")
	}

	// 2. 解析变更文件（只解析暂存区的变更）
	changedFiles, err := git.ParseStagedFiles()
	if err != nil {
		return fmt.Errorf("解析变更文件失败: %w", err)
	}

	// 3. 获取配置的角色和模型
	cfg := config.GetConfig()
	role := cfg.Role
	model := cfg.Model

	// 4. 构建提示词并调用LLM
	prompt := formatter.BuildPrompt(role, changedFiles, diff)
	message, err := llm.GenerateCommitMessage(prompt, model)
	if err != nil {
		return fmt.Errorf("生成提交消息失败: %w", err)
	}

	// 5. 格式化消息
	formattedMessage := formatter.FormatCommitMessage(message)
	
	// 6. 如果指定了issue编号，添加到提交消息中
	if issueNum != "" {
		formattedMessage = fmt.Sprintf("%s (#%s)", formattedMessage, issueNum)
	}
	
	fmt.Println("生成的提交消息:")
	fmt.Println("-------------------")
	fmt.Println(formattedMessage)
	fmt.Println("-------------------")

	// 7. 如果不是dry-run，则执行git commit
	if !dryRun {
		commitArgs := []string{}
		if noVerify {
			commitArgs = append(commitArgs, "--no-verify")
		}

		if err := git.Commit(formattedMessage, commitArgs...); err != nil {
			return fmt.Errorf("git commit 失败: %w", err)
		}
		
		fmt.Println("成功提交变更!")
	} else {
		fmt.Println("Dry run模式，没有执行实际提交")
	}

	return nil
} 