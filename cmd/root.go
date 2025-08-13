package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/samzong/gmc/internal/branch"
	"github.com/samzong/gmc/internal/config"
	"github.com/samzong/gmc/internal/formatter"
	"github.com/samzong/gmc/internal/git"
	"github.com/samzong/gmc/internal/llm"
	"github.com/spf13/cobra"
)

var (
	cfgFile    string
	noVerify   bool
	dryRun     bool
	addAll     bool
	issueNum   string
	autoYes    bool
	configErr  error
	verbose    bool
	branchDesc string
	rootCmd    = &cobra.Command{
		Use:   "gmc",
		Short: "gmc - Git Message Assistant",
		Long: `gmc is a CLI tool that accelerates Git commit efficiency by generating ` +
			`high-quality commit messages using LLM.`,
		Version: fmt.Sprintf("%s (built at %s)", Version, BuildTime),
		RunE: func(cmd *cobra.Command, args []string) error {
			if configErr != nil {
				return fmt.Errorf("configuration error: %w", configErr)
			}
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
	rootCmd.Flags().BoolVarP(&addAll, "all", "a", false,
		"Automatically add all changes to the staging area before committing")
	rootCmd.Flags().StringVar(&issueNum, "issue", "", "Optional issue number")
	rootCmd.Flags().BoolVarP(&autoYes, "yes", "y", false, "Automatically confirm the commit message")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "V", false, "Show detailed git command output")
	rootCmd.Flags().StringVarP(&branchDesc, "branch", "b", "", "Create and switch to a new branch with generated name")

	rootCmd.AddCommand(configCmd)
}

func initConfig() {
	configErr = config.InitConfig(cfgFile)
}

func handleErrors(err error) error {
	if err == nil {
		return nil
	}

	if err.Error() == "No changes detected in the staging area files." {
		fmt.Println("No changes detected in the staging area files.")
		if !addAll {
			fmt.Println("Hint: You can use -a or --all to automatically add all changes to the staging area")
		}
		return nil
	}

	fmt.Fprintln(os.Stderr, "Error:", err)
	return nil
}

func generateAndCommit() error {
	git.Verbose = verbose

	// Handle branch creation
	if err := handleBranchCreation(); err != nil {
		return err
	}

	// Handle staging
	if err := handleStaging(); err != nil {
		return err
	}

	// Get staged changes
	diff, changedFiles, err := getStagedChanges()
	if err != nil {
		return err
	}

	// Generate and process commit message
	return handleCommitFlow(diff, changedFiles)
}

func handleBranchCreation() error {
	if branchDesc == "" {
		return nil
	}

	branchName := branch.GenerateName(branchDesc)
	if branchName == "" {
		return errors.New("invalid branch description: cannot generate branch name")
	}

	fmt.Printf("Creating and switching to branch: %s\n", branchName)
	if err := git.CreateAndSwitchBranch(branchName); err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}
	fmt.Println("Successfully created and switched to new branch!")
	return nil
}

func handleStaging() error {
	if !addAll {
		return nil
	}

	if err := git.AddAll(); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}
	fmt.Println("All changes have been added to the staging area.")
	return nil
}

func getStagedChanges() (string, []string, error) {
	diff, err := git.GetStagedDiff()
	if err != nil {
		return "", nil, fmt.Errorf("failed to get git diff: %w", err)
	}

	if diff == "" {
		return "", nil, errors.New("no changes detected in the staging area files")
	}

	changedFiles, err := git.ParseStagedFiles()
	if err != nil {
		return "", nil, fmt.Errorf("failed to parse staged files: %w", err)
	}

	return diff, changedFiles, nil
}

func handleCommitFlow(diff string, changedFiles []string) error {
	cfg := config.GetConfig()

	for {
		message, err := generateCommitMessage(cfg, changedFiles, diff)
		if err != nil {
			return err
		}

		// Get user confirmation
		action, editedMessage, err := getUserConfirmation(message)
		if err != nil {
			return err
		}

		switch action {
		case "cancel":
			fmt.Println("Commit cancelled by user")
			return nil
		case "regenerate":
			fmt.Println("Regenerating commit message...")
			continue
		case "commit":
			finalMessage := message
			if editedMessage != "" {
				finalMessage = editedMessage
			}
			return performCommit(finalMessage)
		}
	}
}

func generateCommitMessage(cfg *config.Config, changedFiles []string, diff string) (string, error) {
	prompt := formatter.BuildPrompt(cfg.Role, changedFiles, diff)
	message, err := llm.GenerateCommitMessage(prompt, cfg.Model)
	if err != nil {
		return "", fmt.Errorf("failed to generate commit message: %w", err)
	}

	formattedMessage := formatter.FormatCommitMessage(message)
	if issueNum != "" {
		formattedMessage = fmt.Sprintf("%s (#%s)", formattedMessage, issueNum)
	}

	fmt.Println("\nGenerated Commit Message:")
	fmt.Println(formattedMessage)
	return formattedMessage, nil
}

func getUserConfirmation(message string) (string, string, error) {
	if autoYes {
		fmt.Println("Auto-confirming commit message (-y flag is set)")
		return "commit", "", nil
	}

	fmt.Print("\nDo you want to proceed with this commit message? [y/n/r/e] (y=yes, n=no, r=regenerate, e=edit): ")
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return "", "", fmt.Errorf("failed to read user input: %w", err)
	}

	response = strings.ToLower(strings.TrimSpace(response))
	switch response {
	case "n":
		return "cancel", "", nil
	case "r":
		return "regenerate", "", nil
	case "e":
		editedMessage, err := openEditor(message)
		return "commit", editedMessage, err
	case "y", "":
		if response == "" {
			fmt.Println("Using default option (yes)")
		}
		return "commit", "", nil
	default:
		fmt.Println("Invalid input. Commit cancelled")
		return "cancel", "", nil
	}
}

func openEditor(message string) (string, error) {
	fmt.Println("Opening editor to modify commit message...")

	tmpFile, err := os.CreateTemp("", "gmc-commit-")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary file: %w", err)
	}

	tmpFileName := tmpFile.Name()
	defer os.Remove(tmpFileName)

	if _, err := tmpFile.WriteString(message); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("failed to write to temporary file: %w", err)
	}
	tmpFile.Close()

	editor := getEditor()
	cmd := exec.Command(editor, tmpFileName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to open editor: %w", err)
	}

	editedBytes, err := os.ReadFile(tmpFileName)
	if err != nil {
		return "", fmt.Errorf("failed to read edited message: %w", err)
	}

	editedMessage := strings.TrimSpace(string(editedBytes))
	if editedMessage != "" {
		formattedMessage := formatter.FormatCommitMessage(editedMessage)
		if issueNum != "" {
			formattedMessage = fmt.Sprintf("%s (#%s)", formattedMessage, issueNum)
		}
		fmt.Println("Using edited message:")
		fmt.Println(formattedMessage)
		return formattedMessage, nil
	}

	fmt.Println("Empty message provided, using original message")
	return "", nil
}

func getEditor() string {
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor
	}
	if editor := os.Getenv("VISUAL"); editor != "" {
		return editor
	}
	return "vi"
}

func performCommit(message string) error {
	if dryRun {
		fmt.Println("Dry run mode, no actual commit")
		return nil
	}

	commitArgs := []string{}
	if noVerify {
		commitArgs = append(commitArgs, "--no-verify")
	}

	if err := git.Commit(message, commitArgs...); err != nil {
		return fmt.Errorf("failed to commit changes: %w", err)
	}

	fmt.Println("Successfully committed changes!")
	return nil
}
