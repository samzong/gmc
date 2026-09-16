package workflow

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/samzong/gmc/internal/branch"
	"github.com/samzong/gmc/internal/config"
	"github.com/samzong/gmc/internal/formatter"
	"github.com/samzong/gmc/internal/stringsutil"
	"github.com/samzong/gmc/internal/ui"
)

var (
	ErrNoChanges       = errors.New("no changes detected in the staging area files")
	ErrNoCommitSubject = errors.New("LLM returned no usable commit subject")
)

type CommitOptions struct {
	AddAll     bool
	NoVerify   bool
	NoSignoff  bool
	DryRun     bool
	IssueNum   string
	AutoYes    bool
	Verbose    bool
	BranchDesc string
	UserPrompt string
	ErrWriter  io.Writer
	OutWriter  io.Writer
}

type CommitFlow struct {
	git      GitClient
	llm      LLMClient
	cfg      *config.Config
	opts     CommitOptions
	prompter Prompter
}

func NewCommitFlow(git GitClient, llm LLMClient, cfg *config.Config, opts CommitOptions) *CommitFlow {
	return &CommitFlow{
		git:      git,
		llm:      llm,
		cfg:      cfg,
		opts:     opts,
		prompter: &InteractivePrompter{ErrWriter: opts.ErrWriter},
	}
}

func (f *CommitFlow) SetPrompter(p Prompter) {
	f.prompter = p
}

func (f *CommitFlow) Run(fileArgs []string) error {
	if err := f.handleBranchCreation(); err != nil {
		return err
	}

	if len(fileArgs) > 0 {
		return f.handleSelectiveCommit(fileArgs)
	}

	if err := f.handleStaging(); err != nil {
		return err
	}

	changes, err := f.getStagedChanges()
	if err != nil {
		return err
	}

	return f.runCommitLoop(changes, f.performCommit)
}

func (f *CommitFlow) handleBranchCreation() error {
	if f.opts.BranchDesc == "" {
		return nil
	}

	branchName := branch.GenerateName(f.opts.BranchDesc)
	if branchName == "" {
		return errors.New("invalid branch description: cannot generate branch name")
	}

	fmt.Fprintf(f.opts.ErrWriter, "Creating and switching to branch: %s\n", branchName)
	if err := f.git.CreateAndSwitchBranch(branchName); err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}
	fmt.Fprintln(f.opts.ErrWriter, "Successfully created and switched to new branch!")
	return nil
}

func (f *CommitFlow) handleStaging() error {
	if !f.opts.AddAll {
		return nil
	}

	if err := f.git.AddAll(); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}
	fmt.Fprintln(f.opts.ErrWriter, "All changes have been added to the staging area.")
	return nil
}

type stagedChanges struct {
	Diff  string
	Stats string
	Files []string
}

func (f *CommitFlow) getStagedChanges() (stagedChanges, error) {
	diff, err := f.git.GetStagedDiff()
	if err != nil {
		return stagedChanges{}, fmt.Errorf("failed to get git diff: %w", err)
	}

	if diff == "" {
		return stagedChanges{}, ErrNoChanges
	}

	stats, err := f.git.GetStagedDiffStats()
	if err != nil {
		return stagedChanges{}, fmt.Errorf("failed to get git diff stats: %w", err)
	}

	changedFiles, err := f.git.ParseStagedFiles()
	if err != nil {
		return stagedChanges{}, fmt.Errorf("failed to parse staged files: %w", err)
	}

	return stagedChanges{Diff: diff, Stats: stats, Files: changedFiles}, nil
}

func (f *CommitFlow) handleSelectiveCommit(fileArgs []string) error {
	files, err := f.git.ResolveFiles(fileArgs)
	if err != nil {
		return fmt.Errorf("failed to resolve files: %w", err)
	}
	if len(files) == 0 {
		return errors.New("no valid files found")
	}
	staged, modified, untracked, err := f.git.CheckFileStatus(files)
	if err != nil {
		return fmt.Errorf("failed to check file status: %w", err)
	}
	if f.opts.AddAll {
		toStage := slices.Concat(modified, untracked)
		if len(toStage) == 0 && len(staged) == 0 {
			return fmt.Errorf("no changes detected in specified files: %v", files)
		}
		if len(toStage) > 0 {
			if err := f.git.StageFiles(toStage); err != nil {
				return fmt.Errorf("failed to stage files: %w", err)
			}
			fmt.Fprintf(f.opts.ErrWriter, "Staged files: %v\n", toStage)
		}
		staged = slices.Concat(staged, toStage)
	} else if len(staged) == 0 {
		return fmt.Errorf("none specified files staged: %v\nHint: Use 'gmc -a %s' to stage them first",
			files, strings.Join(files, " "))
	}
	diff, err := f.git.GetFilesDiff(staged)
	if err != nil {
		return fmt.Errorf("failed to get diff: %w", err)
	}
	if diff == "" {
		return ErrNoChanges
	}
	return f.runCommitLoop(stagedChanges{Diff: diff, Files: staged}, func(msg string) error {
		return f.performSelectiveCommit(msg, staged)
	})
}

func (f *CommitFlow) runCommitLoop(changes stagedChanges, commitFn func(string) error) error {
	for {
		message, err := f.generateCommitMessage(changes)
		if err != nil {
			return err
		}

		action, editedMessage, err := f.prompter.GetConfirmation(message, f.opts.AutoYes)
		if err != nil {
			return err
		}

		switch action {
		case ActionCancel:
			fmt.Fprintln(f.opts.ErrWriter, "Commit cancelled by user")
			return nil
		case ActionRegenerate:
			fmt.Fprintln(f.opts.ErrWriter, "Regenerating commit message...")
			continue
		case ActionCommit:
			finalMessage := message
			if editedMessage != "" {
				finalMessage = editedMessage
			}
			finalMessage = f.applyIssueSuffix(finalMessage)
			return commitFn(finalMessage)
		}
	}
}

func (f *CommitFlow) generateCommitMessage(changes stagedChanges) (string, error) {
	prompt := formatter.BuildPromptWithConfig(f.cfg, changes.Files, changes.Diff, changes.Stats, f.opts.UserPrompt)

	sp := ui.NewSpinner("Generating commit message...")
	sp.Start()
	message, err := f.llm.GenerateCommitMessage(prompt, f.cfg.Model)
	sp.Stop()

	if err != nil {
		return "", fmt.Errorf("failed to generate commit message: %w", err)
	}

	formattedMessage := formatter.FormatCommitMessageWithConfig(f.cfg, message)
	if formattedMessage == "" {
		return "", ErrNoCommitSubject
	}
	formattedMessage = f.applyIssueSuffix(formattedMessage)

	fmt.Fprintln(f.opts.ErrWriter, "\nGenerated Commit Message:")
	fmt.Fprintln(f.opts.OutWriter, formattedMessage)
	return formattedMessage, nil
}

func (f *CommitFlow) applyIssueSuffix(message string) string {
	if f.opts.IssueNum == "" {
		return message
	}

	issueTag := fmt.Sprintf("(#%s)", f.opts.IssueNum)
	if strings.Contains(message, issueTag) {
		return message
	}

	return fmt.Sprintf("%s %s", message, issueTag)
}

func (f *CommitFlow) buildCommitArgs() []string {
	var args []string
	if f.opts.NoVerify {
		args = append(args, "--no-verify")
	}
	if !f.opts.NoSignoff {
		args = append(args, "-s")
	}
	return args
}

func (f *CommitFlow) performCommit(message string) error {
	if f.opts.DryRun {
		fmt.Fprintln(f.opts.ErrWriter, "Dry run mode, no actual commit")
		return nil
	}

	if err := f.git.Commit(message, f.buildCommitArgs()...); err != nil {
		return fmt.Errorf("failed to commit changes: %w", err)
	}

	fmt.Fprintln(f.opts.ErrWriter, "Successfully committed changes!")
	return nil
}

func (f *CommitFlow) performSelectiveCommit(message string, files []string) error {
	if f.opts.DryRun {
		fmt.Fprintln(f.opts.ErrWriter, "Dry run mode, no actual commit")
		fmt.Fprintf(f.opts.ErrWriter, "Would commit files: %v\n", files)
		return nil
	}

	if err := f.git.CommitFiles(message, files, f.buildCommitArgs()...); err != nil {
		return fmt.Errorf("failed to commit files: %w", err)
	}

	fmt.Fprintf(f.opts.ErrWriter, "Successfully committed files: %v!\n", files)
	return nil
}

func ExtractFilesFromDiff(diff string) []string {
	var files []string

	for _, line := range strings.Split(diff, "\n") {
		if after, found := strings.CutPrefix(line, "+++ b/"); found {
			files = append(files, after)
		} else if after, found := strings.CutPrefix(line, "--- a/"); found {
			files = append(files, after)
		}
	}

	return stringsutil.UniqueStrings(files)
}
