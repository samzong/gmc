package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/samzong/gmc/internal/branch"
	"github.com/samzong/gmc/internal/config"
	"github.com/samzong/gmc/internal/formatter"
	"github.com/samzong/gmc/internal/git"
	"github.com/samzong/gmc/internal/llm"
	"github.com/spf13/cobra"
)

var (
	cfgFile              string
	noVerify             bool
	noSignoff            bool
	dryRun               bool
	addAll               bool
	issueNum             string
	autoYes              bool
	configErr            error
	verbose              bool
	branchDesc           string
	userPrompt           string
	errNoChangesDetected = errors.New("no changes detected in the staging area files")
	rootCmd              = &cobra.Command{
		Use:   "gmc",
		Short: "gmc - Git Message Assistant",
		Long: `gmc is a CLI tool that accelerates Git commit efficiency by generating ` +
			`high-quality commit messages using LLM.`,
		Version: fmt.Sprintf("%s (built at %s)", Version, BuildTime),
		Args:    cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if configErr != nil {
				return fmt.Errorf("configuration error: %w", configErr)
			}
			return handleErrors(generateAndCommit(args))
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
	rootCmd.Flags().BoolVar(&noSignoff, "no-signoff", false, "Skip signing the commit (DCO signoff)")
	rootCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Generate message only, do not commit")
	rootCmd.Flags().BoolVarP(&addAll, "all", "a", false,
		"Stage files before committing (all files if none specified, or only specified files)")
	rootCmd.Flags().StringVar(&issueNum, "issue", "", "Optional issue number")
	rootCmd.Flags().BoolVarP(&autoYes, "yes", "y", false, "Automatically confirm the commit message")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "V", false, "Show detailed git command output")
	rootCmd.Flags().StringVarP(&branchDesc, "branch", "b", "", "Create and switch to a new branch with generated name")
	rootCmd.Flags().StringVarP(&userPrompt, "prompt", "p", "",
		"Additional context or instructions for commit message generation")

	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(initCmd)
}

func initConfig() {
	configErr = config.InitConfig(cfgFile)
}

func handleErrors(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, errNoChangesDetected) {
		fmt.Fprintln(os.Stderr, "No changes detected in the staging area files.")
		if !addAll {
			fmt.Fprintln(os.Stderr, "Hint: You can use -a or --all to automatically add all changes to the staging area.")
		}
		return err
	}

	fmt.Fprintf(os.Stderr, "gmc: %v\n", err)
	return err
}

func generateAndCommit(fileArgs []string) error {
	git.Verbose = verbose

	// Handle branch creation
	if err := handleBranchCreation(); err != nil {
		return err
	}

	// Check if selective file mode
	if len(fileArgs) > 0 {
		return handleSelectiveCommit(fileArgs)
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

	fmt.Fprintf(os.Stderr, "Creating and switching to branch: %s\n", branchName)
	if err := git.CreateAndSwitchBranch(branchName); err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}
	fmt.Fprintln(os.Stderr, "Successfully created and switched to new branch!")
	return nil
}

func handleStaging() error {
	if !addAll {
		return nil
	}

	if err := git.AddAll(); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}
	fmt.Fprintln(os.Stderr, "All changes have been added to the staging area.")
	return nil
}

func getStagedChanges() (string, []string, error) {
	diff, err := git.GetStagedDiff()
	if err != nil {
		return "", nil, fmt.Errorf("failed to get git diff: %w", err)
	}

	if diff == "" {
		return "", nil, errNoChangesDetected
	}

	changedFiles, err := git.ParseStagedFiles()
	if err != nil {
		return "", nil, fmt.Errorf("failed to parse staged files: %w", err)
	}

	return diff, changedFiles, nil
}

func handleCommitFlow(diff string, changedFiles []string) error {
	cfg := config.GetConfig()

	proceed, err := ensureLLMConfigured(cfg, os.Stdin, os.Stderr, runInitWizard)
	if err != nil {
		return err
	}
	if !proceed {
		return nil
	}
	cfg = config.GetConfig()

	return runCommitFlow(cfg, changedFiles, diff, performCommit)
}

func generateCommitMessage(cfg *config.Config, changedFiles []string, diff string, userPrompt string) (string, error) {
	prompt := formatter.BuildPrompt(cfg.Role, changedFiles, diff, userPrompt)
	message, err := llm.GenerateCommitMessage(prompt, cfg.Model)
	if err != nil {
		return "", fmt.Errorf("failed to generate commit message: %w", err)
	}

	formattedMessage := formatter.FormatCommitMessage(message)
	if issueNum != "" {
		formattedMessage = fmt.Sprintf("%s (#%s)", formattedMessage, issueNum)
	}

	fmt.Fprintln(os.Stderr, "\nGenerated Commit Message:")
	fmt.Println(formattedMessage)
	return formattedMessage, nil
}

func getUserConfirmation(message string) (string, string, error) {
	if autoYes {
		fmt.Fprintln(os.Stderr, "Auto-confirming commit message (-y flag is set)")
		return "commit", "", nil
	}

	if !isatty.IsTerminal(os.Stdin.Fd()) && !isatty.IsCygwinTerminal(os.Stdin.Fd()) {
		return "", "", errors.New("stdin is not a terminal, use --yes to skip interactive confirmation")
	}

	fmt.Fprint(os.Stderr, "\nDo you want to proceed with this commit message? [y/n/r/e] (y=yes, n=no, r=regenerate, e=edit): ")
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
			fmt.Fprintln(os.Stderr, "Using default option (yes)")
		}
		return "commit", "", nil
	default:
		fmt.Fprintln(os.Stderr, "Invalid input. Commit cancelled")
		return "cancel", "", nil
	}
}

func openEditor(message string) (string, error) {
	fmt.Fprintln(os.Stderr, "Opening editor to modify commit message...")

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
		fmt.Fprintln(os.Stderr, "Using edited message:")
		fmt.Fprintln(os.Stderr, formattedMessage)
		return formattedMessage, nil
	}

	fmt.Fprintln(os.Stderr, "Empty message provided, using original message")
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

func handleSelectiveCommit(fileArgs []string) error {
	// Resolve and validate file paths
	files, err := git.ResolveFiles(fileArgs)
	if err != nil {
		return fmt.Errorf("failed to resolve files: %w", err)
	}

	if len(files) == 0 {
		return errors.New("no valid files found")
	}

	// Check mode based on -a flag
	if addAll {
		return stageAndCommitFiles(files)
	}
	return commitStagedFiles(files)
}

func stageAndCommitFiles(files []string) error {
	// Check which files have changes
	staged, modified, untracked, err := git.CheckFileStatus(files)
	if err != nil {
		return fmt.Errorf("failed to check file status: %w", err)
	}

	// Determine files to stage
	toStage := make([]string, 0, len(modified)+len(untracked))
	toStage = append(toStage, modified...)
	toStage = append(toStage, untracked...)
	if len(toStage) == 0 && len(staged) == 0 {
		return fmt.Errorf("no changes detected in specified files: %v", files)
	}

	// Stage files if needed
	if len(toStage) > 0 {
		if err := git.StageFiles(toStage); err != nil {
			return fmt.Errorf("failed to stage files: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Staged files: %v\n", toStage)
	}

	// Get all files to commit (staged + newly staged)
	allFiles := make([]string, 0, len(staged)+len(toStage))
	allFiles = append(allFiles, staged...)
	allFiles = append(allFiles, toStage...)

	// Get diff and generate commit message
	diff, err := git.GetFilesDiff(allFiles)
	if err != nil {
		return fmt.Errorf("failed to get diff: %w", err)
	}

	if diff == "" {
		return errNoChangesDetected
	}

	// Generate and process commit message
	return handleSelectiveCommitFlow(diff, allFiles)
}

func commitStagedFiles(files []string) error {
	// Check which specified files are staged
	staged, _, _, err := git.CheckFileStatus(files)
	if err != nil {
		return fmt.Errorf("failed to check file status: %w", err)
	}

	if len(staged) == 0 {
		fileNames := strings.Join(files, " ")
		return fmt.Errorf("none specified files staged: %v\nHint: Use 'gmc -a %s' to stage them first", files, fileNames)
	}

	// Get diff for staged files
	diff, err := git.GetFilesDiff(staged)
	if err != nil {
		return fmt.Errorf("failed to get diff: %w", err)
	}

	if diff == "" {
		return errNoChangesDetected
	}

	// Generate and process commit message
	return handleSelectiveCommitFlow(diff, staged)
}

func handleSelectiveCommitFlow(diff string, files []string) error {
	cfg := config.GetConfig()

	proceed, err := ensureLLMConfigured(cfg, os.Stdin, os.Stderr, runInitWizard)
	if err != nil {
		return err
	}
	if !proceed {
		return nil
	}
	cfg = config.GetConfig()

	return runCommitFlow(cfg, files, diff, func(message string) error {
		return performSelectiveCommit(message, files)
	})
}

// buildCommitArgs constructs the git commit arguments based on flags
func buildCommitArgs() []string {
	commitArgs := []string{}
	if noVerify {
		commitArgs = append(commitArgs, "--no-verify")
	}
	if !noSignoff {
		commitArgs = append(commitArgs, "-s")
	}
	return commitArgs
}

func performCommit(message string) error {
	if dryRun {
		fmt.Fprintln(os.Stderr, "Dry run mode, no actual commit")
		return nil
	}

	commitArgs := buildCommitArgs()

	if err := git.Commit(message, commitArgs...); err != nil {
		return fmt.Errorf("failed to commit changes: %w", err)
	}

	fmt.Fprintln(os.Stderr, "Successfully committed changes!")
	return nil
}

func performSelectiveCommit(message string, files []string) error {
	if dryRun {
		fmt.Fprintln(os.Stderr, "Dry run mode, no actual commit")
		fmt.Fprintf(os.Stderr, "Would commit files: %v\n", files)
		return nil
	}

	commitArgs := buildCommitArgs()

	if err := git.CommitFiles(message, files, commitArgs...); err != nil {
		return fmt.Errorf("failed to commit files: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Successfully committed files: %v!\n", files)
	return nil
}

func runCommitFlow(cfg *config.Config, files []string, diff string, commitExec func(string) error) error {
	for {
		message, err := generateCommitMessage(cfg, files, diff, userPrompt)
		if err != nil {
			return err
		}

		action, editedMessage, err := getUserConfirmation(message)
		if err != nil {
			return err
		}

		switch action {
		case "cancel":
			fmt.Fprintln(os.Stderr, "Commit cancelled by user")
			return nil
		case "regenerate":
			fmt.Fprintln(os.Stderr, "Regenerating commit message...")
			continue
		case "commit":
			finalMessage := message
			if editedMessage != "" {
				finalMessage = editedMessage
			}
			return commitExec(finalMessage)
		}
	}
}
