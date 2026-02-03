package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/samzong/gmc/internal/config"
	"github.com/samzong/gmc/internal/formatter"
	"github.com/samzong/gmc/internal/git"
	"github.com/samzong/gmc/internal/llm"
	"github.com/samzong/gmc/internal/ui"
	"github.com/samzong/gmc/internal/workflow"
	"github.com/spf13/cobra"
)

var (
	cfgFile        string
	noVerify       bool
	noSignoff      bool
	dryRun         bool
	addAll         bool
	issueNum       string
	autoYes        bool
	configErr      error
	verbose        bool
	branchDesc     string
	userPrompt     string
	timeoutSeconds int
	debug          bool
	rootCmd        = &cobra.Command{
		Use:   "gmc",
		Short: "gmc - Git Message Assistant",
		Long: `gmc is a CLI tool that accelerates Git commit efficiency by generating ` +
			`high-quality commit messages using LLM.`,
		Version:       fmt.Sprintf("%s (built at %s)", Version, BuildTime),
		Args:          cobra.ArbitraryArgs,
		RunE:          runRoot,
		SilenceErrors: false,
		SilenceUsage:  true,
	}
)

func RootCmd() *cobra.Command {
	return rootCmd
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(
		&cfgFile, "config", "", "Config file (default: $XDG_CONFIG_HOME/gmc/config.yaml)")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug output")

	rootCmd.Flags().BoolP("version", "V", false, "version for gmc")
	rootCmd.Flags().BoolVar(&noVerify, "no-verify", false, "Skip pre-commit hooks")
	rootCmd.Flags().BoolVar(&noSignoff, "no-signoff", false, "Skip signing the commit (DCO signoff)")
	rootCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Generate message only, do not commit")
	rootCmd.Flags().BoolVarP(&addAll, "all", "a", false,
		"Stage files before committing (all files if none specified, or only specified files)")
	rootCmd.Flags().StringVar(&issueNum, "issue", "", "Optional issue number")
	rootCmd.Flags().BoolVarP(&autoYes, "yes", "y", false, "Automatically confirm the commit message")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show detailed git command output")
	rootCmd.Flags().StringVarP(&branchDesc, "branch", "b", "", "Create and switch to a new branch with generated name")
	rootCmd.Flags().StringVarP(&userPrompt, "prompt", "p", "",
		"Additional context or instructions for commit message generation")
	rootCmd.Flags().IntVar(&timeoutSeconds, "timeout", 30, "LLM request timeout in seconds")

	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(initCmd)
}

func initConfig() {
	configErr = config.InitConfig(cfgFile)
}

func runRoot(cmd *cobra.Command, args []string) error {
	if configErr != nil {
		return handleErrors(fmt.Errorf("configuration error: %w", configErr), addAll)
	}
	return handleErrors(generateAndCommit(cmd.InOrStdin(), args), addAll)
}

func handleErrors(err error, addAllFlag bool) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, workflow.ErrNoChanges) {
		msg := "No changes detected in the staging area files."
		if !addAllFlag {
			msg += "\nHint: You can use -a or --all to automatically add all changes to the staging area."
		}
		return userFacingError{msg: msg, err: err}
	}

	return userFacingError{msg: fmt.Sprintf("gmc: %v", err), err: err}
}

type userFacingError struct {
	msg string
	err error
}

func (e userFacingError) Error() string {
	return e.msg
}

func (e userFacingError) Unwrap() error {
	return e.err
}

func generateAndCommit(in io.Reader, fileArgs []string) error {
	gitClient := git.NewClient(git.Options{Verbose: verbose})
	llmClient := llm.NewClient(llm.Options{Timeout: time.Duration(timeoutSeconds) * time.Second})

	if len(fileArgs) == 1 && fileArgs[0] == "-" {
		return handleStdinDiff(in, llmClient)
	}

	cfg, proceed, err := ensureConfiguredAndGetConfig(nil, in, errWriter(), runInitWizard)
	if err != nil {
		return err
	}
	if !proceed {
		return nil
	}

	opts := workflow.CommitOptions{
		AddAll:     addAll,
		NoVerify:   noVerify,
		NoSignoff:  noSignoff,
		DryRun:     dryRun,
		IssueNum:   issueNum,
		AutoYes:    autoYes,
		Verbose:    verbose,
		BranchDesc: branchDesc,
		UserPrompt: userPrompt,
		ErrWriter:  errWriter(),
		OutWriter:  outWriter(),
	}

	flow := workflow.NewCommitFlow(gitClient, llmClient, cfg, opts)
	flow.SetPrompter(&workflow.InteractivePrompter{
		ErrWriter: errWriter(),
		Stdin:     in,
		Cfg:       cfg,
	})

	return flow.Run(fileArgs)
}

func handleStdinDiff(in io.Reader, llmClient *llm.Client) error {
	if f, ok := in.(*os.File); ok {
		if isatty.IsTerminal(f.Fd()) {
			return errors.New("no input from stdin: use 'git diff | gmc -' or 'gmc - < diff.txt'")
		}
	}

	if in == nil {
		return errors.New("no input from stdin: use 'git diff | gmc -' or 'gmc - < diff.txt'")
	}

	data, err := io.ReadAll(in)
	if err != nil {
		return fmt.Errorf("failed to read stdin: %w", err)
	}

	diff := strings.TrimSpace(string(data))
	if diff == "" {
		return errors.New("empty diff received from stdin")
	}

	if debug {
		fmt.Fprintf(errWriter(), "[debug] Read %d bytes from stdin\n", len(diff))
	}

	changedFiles := workflow.ExtractFilesFromDiff(diff)

	cfg, proceed, err := ensureConfiguredAndGetConfig(nil, in, errWriter(), runInitWizard)
	if err != nil {
		return err
	}
	if !proceed {
		return nil
	}

	message, err := generateStdinMessage(llmClient, cfg, changedFiles, diff)
	if err != nil {
		return err
	}

	fmt.Fprintln(errWriter(), "\n[stdin mode: message only, no commit]")
	fmt.Fprintln(outWriter(), message)
	return nil
}

func generateStdinMessage(
	llmClient *llm.Client, cfg *config.Config, changedFiles []string, diff string,
) (string, error) {
	prompt := formatter.BuildPromptWithConfig(cfg, changedFiles, diff, userPrompt)

	sp := ui.NewSpinner("Generating commit message...")
	sp.Start()
	message, err := llmClient.GenerateCommitMessage(prompt, cfg.Model)
	sp.Stop()

	if err != nil {
		return "", fmt.Errorf("failed to generate commit message: %w", err)
	}

	formattedMessage := formatter.FormatCommitMessageWithConfig(cfg, message)
	if issueNum != "" {
		formattedMessage = fmt.Sprintf("%s (#%s)", formattedMessage, issueNum)
	}

	return formattedMessage, nil
}

func ensureConfiguredAndGetConfig(
	cfg *config.Config,
	in io.Reader,
	out io.Writer,
	initRunner func(io.Reader, io.Writer, *config.Config) error,
) (*config.Config, bool, error) {
	proceed, err := ensureLLMConfigured(cfg, in, out, initRunner)
	if err != nil || !proceed {
		return nil, proceed, err
	}
	updatedCfg, cfgErr := config.GetConfig()
	if cfgErr != nil {
		return nil, false, cfgErr
	}
	return updatedCfg, true, nil
}
