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
	"os/exec"
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
			fmt.Print("\nDo you want to proceed with this commit message? [y/n/r/e] (y=yes, n=no, r=regenerate, e=edit): ")
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
			case "e":
				fmt.Println("Opening editor to modify commit message...")

				tmpFile, err := os.CreateTemp("", "gmc-commit-")
				if err != nil {
					return fmt.Errorf("Failed to create temporary file: %w", err)
				}

				tmpFileName := tmpFile.Name()
				defer os.Remove(tmpFileName)

				if _, err := tmpFile.WriteString(formattedMessage); err != nil {
					tmpFile.Close()
					return fmt.Errorf("Failed to write to temporary file: %w", err)
				}
				tmpFile.Close()

				editor := os.Getenv("EDITOR")
				if editor == "" {
					editor = os.Getenv("VISUAL")
				}
				if editor == "" {
					editor = "vi" // use vi for edit
				}

				cmd := exec.Command(editor, tmpFileName)
				cmd.Stdin = os.Stdin
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr

				if err := cmd.Run(); err != nil {
					return fmt.Errorf("Failed to open editor: %w", err)
				}

				editedBytes, err := os.ReadFile(tmpFileName)
				if err != nil {
					return fmt.Errorf("Failed to read edited message: %w", err)
				}

				editedMessage := strings.TrimSpace(string(editedBytes))
				if editedMessage != "" {
					formattedMessage = formatter.FormatCommitMessage(editedMessage)
					if issueNum != "" {
						formattedMessage = fmt.Sprintf("%s (#%s)", formattedMessage, issueNum)
					}
					fmt.Println("Using edited message:")
					fmt.Println(formattedMessage)
				} else {
					fmt.Println("Empty message provided, using original message")
				}
			case "y", "":
				if response == "" {
					fmt.Println("Using default option (yes)")
				}
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
