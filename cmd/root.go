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
	addAll    bool
	issueNum  string
	rootCmd   = &cobra.Command{
		Use:   "gma",
		Short: "GMA - Git Message Assistant",
		Long: `GMA is a CLI tool that accelerates Git commit efficiency by generating high-quality commit messages using LLM.
With GMA, you can complete git add and commit operations with a single command, reducing the mental burden of developers when submitting code.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleErrors(generateAndCommit())
		},
		SilenceErrors: true,
		SilenceUsage:  true,
	}
)

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "Configuration file path (default is $HOME/.gma.yaml)")
	rootCmd.Flags().BoolVar(&noVerify, "no-verify", false, "Skip pre-commit hooks")
	rootCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Generate message only, do not commit")
	rootCmd.Flags().BoolVarP(&addAll, "all", "a", false, "Automatically add all changes to the staging area before committing")
	rootCmd.Flags().StringVar(&issueNum, "issue", "", "Optional issue number")

	rootCmd.AddCommand(configCmd)
}

func initConfig() {
	config.InitConfig(cfgFile)
}

func handleErrors(err error) error {
	if err != nil {
		if err.Error() == "No changes detected in the staging area files." {
			fmt.Println("No changes detected in the staging area files.")
			if !addAll {
				fmt.Println("Hint: You can use -a or --all to automatically add all changes to the staging area")
			}
			return nil
		} else if err.Error() == "No changes detected in the staging area files." {
			fmt.Println("No changes detected in the staging area files.")
			if !addAll {
				fmt.Println("Hint: You can use -a or --all to automatically add all changes to the staging area")
			}
			return nil
		}
		
		fmt.Fprintln(os.Stderr, "Error:", err)
		return nil
	}
	return nil
}

func generateAndCommit() error {
	if addAll {
		if err := git.AddAll(); err != nil {
			return fmt.Errorf("git add 失败: %w", err)
		}
		fmt.Println("All changes have been added to the staging area.")
	}

	diff, err := git.GetStagedDiff()
	if err != nil {
		return fmt.Errorf("Failed to get git diff: %w", err)
	}

	if diff == "" {
		return fmt.Errorf("No changes detected in the staging area files.")
	}

	changedFiles, err := git.ParseStagedFiles()
	if err != nil {
		return fmt.Errorf("Failed to parse change file: %w", err)
	}

	cfg := config.GetConfig()
	role := cfg.Role
	model := cfg.Model

	prompt := formatter.BuildPrompt(role, changedFiles, diff)
	message, err := llm.GenerateCommitMessage(prompt, model)
	if err != nil {
		return fmt.Errorf("Failed to generate commit message: %w", err)
	}

	formattedMessage := formatter.FormatCommitMessage(message)
	
	if issueNum != "" {
		formattedMessage = fmt.Sprintf("%s (#%s)", formattedMessage, issueNum)
	}
	
	fmt.Println("Generated Commit Message:")
	fmt.Println("-------------------")
	fmt.Println(formattedMessage)
	fmt.Println("-------------------")

	if !dryRun {
		commitArgs := []string{}
		if noVerify {
			commitArgs = append(commitArgs, "--no-verify")
		}

		if err := git.Commit(formattedMessage, commitArgs...); err != nil {
			return fmt.Errorf("Failed to commit changes: %w", err)
		}
		
		fmt.Println("Successfully committed changes!")
	} else {
		fmt.Println("Dry run mode, no actual commit")
	}

	return nil
} 