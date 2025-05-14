package cmd

import (
	"bufio"
	"fmt"
	"github.com/samzong/gmc/internal/config"
	"github.com/samzong/gmc/internal/formatter"
	"github.com/samzong/gmc/internal/git"
	"github.com/samzong/gmc/internal/llm"
	"github.com/spf13/cobra"
	"os"
	"strings"
)

var (
	cfgFile  string
	noVerify bool
	dryRun   bool
	addAll   bool
	issueNum string
	autoYes  bool
	rootCmd  = &cobra.Command{
		Use:     "gmc",
		Short:   "gmc - Git Message Assistant",
		Long:    `gmc is a CLI tool that accelerates Git commit efficiency by generating high-quality commit messages using LLM.`,
		Version: fmt.Sprintf("%s (built at %s)", Version, BuildTime),
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

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "Configuration file path (default is $HOME/.gmc.yaml)")
	rootCmd.Flags().BoolVar(&noVerify, "no-verify", false, "Skip pre-commit hooks")
	rootCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Generate message only, do not commit")
	rootCmd.Flags().BoolVarP(&addAll, "all", "a", false, "Automatically add all changes to the staging area before committing")
	rootCmd.Flags().StringVar(&issueNum, "issue", "", "Optional issue number")
	rootCmd.Flags().BoolVarP(&autoYes, "yes", "y", false, "Automatically confirm the commit message")

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
			return fmt.Errorf("git add failed: %w", err)
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

	for {
		prompt := formatter.BuildPrompt(role, changedFiles, diff)
		message, err := llm.GenerateCommitMessage(prompt, model)
		if err != nil {
			return fmt.Errorf("Failed to generate commit message: %w", err)
		}

		formattedMessage := formatter.FormatCommitMessage(message)

		if issueNum != "" {
			formattedMessage = fmt.Sprintf("%s (#%s)", formattedMessage, issueNum)
		}

		fmt.Println("\nGenerated Commit Message:")
		fmt.Println(formattedMessage)

		if autoYes {
			fmt.Println("Auto-confirming commit message (-y flag is set)")
		} else {
			fmt.Print("\nDo you want to proceed with this commit message? [y/n/r] (y=yes, n=no, r=regenerate): ")
			reader := bufio.NewReader(os.Stdin)
			response, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("Failed to read user input: %w", err)
			}

			response = strings.ToLower(strings.TrimSpace(response))
			switch response {
			case "n":
				fmt.Println("Commit cancelled by user")
				return nil
			case "r":
				fmt.Println("Regenerating commit message...")
				continue
			case "y":
				// Continue with commit
			default:
				fmt.Println("Invalid input. Commit cancelled")
				return nil
			}
		}

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
}
