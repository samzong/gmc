package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/samzong/gmc/internal/config"
	"github.com/samzong/gmc/internal/git"
	"github.com/samzong/gmc/internal/llm"
	"github.com/samzong/gmc/internal/version"
	"github.com/spf13/cobra"
)

var (
	tagAutoYes bool

	// isStdinTerminal is a function to check if stdin is a terminal.
	// It can be overridden in tests.
	isStdinTerminal = func() bool {
		return isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())
	}

	tagCmd = &cobra.Command{
		Use:   "tag",
		Short: "Suggest and create a semantic version tag based on commits since the last release",
		Long: `Analyze commits since the latest git tag, recommend the next semantic version, ` +
			`and optionally create the tag when confirmed.

Examples:
  gmc tag          # Analyze commits and interactively create a tag
  gmc tag --yes    # Auto-confirm tag creation with the suggested version`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runTagCommand()
		},
	}
)

func init() {
	tagCmd.Flags().BoolVarP(
		&tagAutoYes,
		"yes",
		"y",
		false,
		"Automatically confirm tag creation with the suggested version",
	)
	rootCmd.AddCommand(tagCmd)
}

func runTagCommand() error {
	gitClient := git.NewClient(git.Options{Verbose: verbose})
	llmClient := llm.NewClient(llm.Options{Timeout: time.Duration(timeoutSeconds) * time.Second})

	lastTag, commits, err := collectTagContext(gitClient)
	if err != nil {
		return err
	}
	if len(commits) == 0 {
		printNoCommitsSinceLastTag(lastTag)
		return nil
	}

	baseVersion, displayTag, err := resolveBaseVersion(lastTag)
	if err != nil {
		return err
	}

	printCommitSummary(displayTag, commits)

	finalVersion, finalReason, source, err := pickTagSuggestion(baseVersion, commits, llmClient)
	if err != nil {
		return err
	}
	fmt.Fprintf(outWriter(), "Suggested version (%s): %s\n", source, finalVersion.String())
	if strings.TrimSpace(finalReason) != "" {
		fmt.Fprintf(outWriter(), "Reason: %s\n", finalReason)
	}
	fmt.Fprintln(outWriter())

	if msg, skip := shouldSkipTagCreation(lastTag, finalVersion, baseVersion); skip {
		fmt.Fprintln(outWriter(), msg)
		return nil
	}

	confirmed, err := confirmTagCreation(finalVersion.String())
	if err != nil {
		return fmt.Errorf("failed to read confirmation: %w", err)
	}

	if !confirmed {
		fmt.Fprintln(outWriter(), "Tag creation cancelled.")
		return nil
	}

	tagMessage := "Release " + finalVersion.String()
	if strings.TrimSpace(finalReason) != "" {
		tagMessage = fmt.Sprintf("%s: %s", tagMessage, finalReason)
	}

	if err := gitClient.CreateAnnotatedTag(finalVersion.String(), tagMessage); err != nil {
		return fmt.Errorf("failed to create tag: %w", err)
	}

	fmt.Fprintf(outWriter(), "Tag %s created successfully.\n", finalVersion.String())
	fmt.Fprintf(outWriter(), "Hint: run `git push origin %s` to share the tag.\n", finalVersion.String())
	return nil
}

func collectTagContext(gitClient *git.Client) (string, []git.CommitInfo, error) {
	if err := gitClient.CheckGitRepository(); err != nil {
		return "", nil, fmt.Errorf("tagging failed: %w", err)
	}

	lastTag, err := gitClient.GetLatestTag()
	if err != nil {
		return "", nil, fmt.Errorf("failed to determine latest tag: %w", err)
	}

	commits, err := gitClient.GetCommitsSinceTag(lastTag)
	if err != nil {
		return "", nil, fmt.Errorf("failed to collect commits: %w", err)
	}

	return lastTag, commits, nil
}

func printNoCommitsSinceLastTag(lastTag string) {
	if lastTag == "" {
		fmt.Fprintln(outWriter(), "No commits found in the repository; nothing to tag yet.")
		return
	}
	fmt.Fprintf(outWriter(), "No new commits since %s; no tag created.\n", lastTag)
}

func resolveBaseVersion(lastTag string) (version.SemVer, string, error) {
	baseTag := lastTag
	if baseTag == "" {
		baseTag = "v0.0.0"
	}

	baseVersion, err := version.ParseSemVer(baseTag)
	if err != nil {
		return version.SemVer{}, "", fmt.Errorf("failed to parse base version %s: %w", baseTag, err)
	}

	displayTag := baseTag
	if lastTag == "" {
		displayTag = "initial commit"
	}
	return baseVersion, displayTag, nil
}

func printCommitSummary(displayTag string, commits []git.CommitInfo) {
	fmt.Fprintf(outWriter(), "Commits since %s (%d total):\n", displayTag, len(commits))
	for _, commit := range commits {
		fmt.Fprintf(outWriter(), "  - %s\n", commit.Message)
	}
	fmt.Fprintln(outWriter())
}

func pickTagSuggestion(
	baseVersion version.SemVer, commits []git.CommitInfo, llmClient *llm.Client,
) (version.SemVer, string, string, error) {
	ruleResult := version.SuggestWithRules(baseVersion, commits)
	finalVersion := ruleResult.NextVersion
	finalReason := ruleResult.Reason
	source := "rule engine"

	cfg, err := config.GetConfig()
	if err != nil {
		return version.SemVer{}, "", "", err
	}
	if cfg.APIKey == "" {
		return finalVersion, finalReason, source, nil
	}

	llmVersion, llmReason, ok := selectVersionSuggestion(
		baseVersion, ruleResult.NextVersion, commits, cfg.Model, llmClient,
	)
	if !ok {
		return finalVersion, finalReason, source, nil
	}

	finalVersion = llmVersion
	if strings.TrimSpace(llmReason) != "" {
		finalReason = llmReason
	}
	return finalVersion, finalReason, "LLM", nil
}

func selectVersionSuggestion(
	baseVersion version.SemVer,
	ruleVersion version.SemVer,
	commits []git.CommitInfo,
	model string,
	llmClient *llm.Client,
) (version.SemVer, string, bool) {
	commitSummaries := buildCommitSummaries(commits)
	llmVersionStr, llmReason, err := llmClient.SuggestVersion(baseVersion.String(), commitSummaries, model)
	if err != nil {
		fmt.Fprintf(errWriter(), "Warning: LLM version suggestion failed: %v\n", err)
		return version.SemVer{}, "", false
	}

	llmVersion, parseErr := version.ParseSemVer(llmVersionStr)
	switch {
	case parseErr != nil:
		fmt.Fprintf(errWriter(), "Warning: Invalid LLM version suggestion %q: %v\n", llmVersionStr, parseErr)
		return version.SemVer{}, "", false
	case llmVersion.LessThan(ruleVersion):
		fmt.Fprintf(
			errWriter(),
			"Warning: LLM suggested %s which is lower than rule-based %s; keeping rule result.\n",
			llmVersion.String(),
			ruleVersion.String(),
		)
		return version.SemVer{}, "", false
	default:
		return llmVersion, llmReason, true
	}
}

func buildCommitSummaries(commits []git.CommitInfo) []string {
	commitSummaries := make([]string, 0, len(commits))
	for _, commit := range commits {
		summary := strings.TrimSpace(commit.Message)
		lowerSummary := strings.ToLower(summary)
		lowerBody := strings.ToLower(commit.Body)

		if strings.Contains(lowerSummary, "breaking change") ||
			strings.Contains(lowerBody, "breaking change") ||
			strings.Contains(summary, "!:") {
			summary += " (breaking change?)"
		}

		commitSummaries = append(commitSummaries, summary)
	}
	return commitSummaries
}

func shouldSkipTagCreation(lastTag string, finalVersion, baseVersion version.SemVer) (string, bool) {
	if !finalVersion.Equal(baseVersion) {
		return "", false
	}
	if lastTag != "" {
		return "No version bump recommended. No tag created.", true
	}
	return fmt.Sprintf("Suggested version matches base version %s; skipping tag creation.", baseVersion.String()), true
}

func confirmTagCreation(tag string) (bool, error) {
	if tagAutoYes {
		fmt.Fprintln(errWriter(), "Auto-confirming tag creation (-y flag is set)")
		return true, nil
	}

	if !isStdinTerminal() {
		return false, errors.New("stdin is not a terminal, use --yes to skip interactive confirmation")
	}

	fmt.Fprintf(errWriter(), "Create tag %s? [y/N]: ", tag)
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		if err == io.EOF {
			return false, nil
		}
		return false, err
	}

	answer := strings.TrimSpace(strings.ToLower(input))
	return answer == "y" || answer == "yes", nil
}
