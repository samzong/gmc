package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/samzong/gmc/internal/config"
	"github.com/samzong/gmc/internal/git"
	"github.com/samzong/gmc/internal/llm"
	"github.com/samzong/gmc/internal/version"
	"github.com/spf13/cobra"
)

var (
	tagAutoYes bool

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
	if err := git.CheckGitRepository(); err != nil {
		return fmt.Errorf("tagging failed: %w", err)
	}

	lastTag, err := git.GetLatestTag()
	if err != nil {
		return fmt.Errorf("failed to determine latest tag: %w", err)
	}

	commits, err := git.GetCommitsSinceTag(lastTag)
	if err != nil {
		return fmt.Errorf("failed to collect commits: %w", err)
	}

	if len(commits) == 0 {
		if lastTag == "" {
			fmt.Println("No commits found in the repository; nothing to tag yet.")
		} else {
			fmt.Printf("No new commits since %s; no tag created.\n", lastTag)
		}
		return nil
	}

	baseTag := lastTag
	if baseTag == "" {
		baseTag = "v0.0.0"
	}

	baseVersion, err := version.ParseSemVer(baseTag)
	if err != nil {
		return fmt.Errorf("failed to parse base version %s: %w", baseTag, err)
	}

	displayTag := baseTag
	if lastTag == "" {
		displayTag = "initial commit"
	}

	fmt.Printf("Commits since %s (%d total):\n", displayTag, len(commits))
	for _, commit := range commits {
		fmt.Printf("  - %s\n", commit.Message)
	}
	fmt.Println()

	ruleResult := version.SuggestWithRules(baseVersion, commits)
	finalVersion := ruleResult.NextVersion
	finalReason := ruleResult.Reason
	source := "rule engine"

	cfg := config.GetConfig()
	if cfg.APIKey != "" {
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

		llmVersionStr, llmReason, err := llm.SuggestVersion(baseVersion.String(), commitSummaries, cfg.Model)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: LLM version suggestion failed: %v\n", err)
		} else {
			llmVersion, parseErr := version.ParseSemVer(llmVersionStr)

			switch {
			case parseErr != nil:
				fmt.Fprintf(
					os.Stderr,
					"Warning: Invalid LLM version suggestion %q: %v\n",
					llmVersionStr,
					parseErr,
				)
			case llmVersion.LessThan(ruleResult.NextVersion):
				fmt.Fprintf(
					os.Stderr,
					"Warning: LLM suggested %s which is lower than rule-based %s; keeping rule result.\n",
					llmVersion.String(),
					ruleResult.NextVersion.String(),
				)
			default:
				finalVersion = llmVersion
				if strings.TrimSpace(llmReason) != "" {
					finalReason = llmReason
				}
				source = "LLM"
			}
		}
	}

	fmt.Printf("Suggested version (%s): %s\n", source, finalVersion.String())
	if strings.TrimSpace(finalReason) != "" {
		fmt.Printf("Reason: %s\n", finalReason)
	}
	fmt.Println()

	if lastTag != "" && finalVersion.Equal(baseVersion) {
		fmt.Println("No version bump recommended. No tag created.")
		return nil
	}

	if lastTag == "" && finalVersion.Equal(baseVersion) {
		fmt.Printf("Suggested version matches base version %s; skipping tag creation.\n", baseVersion.String())
		return nil
	}

	confirmed, err := confirmTagCreation(finalVersion.String())
	if err != nil {
		return fmt.Errorf("failed to read confirmation: %w", err)
	}

	if !confirmed {
		fmt.Println("Tag creation cancelled.")
		return nil
	}

	tagMessage := "Release " + finalVersion.String()
	if strings.TrimSpace(finalReason) != "" {
		tagMessage = fmt.Sprintf("%s: %s", tagMessage, finalReason)
	}

	if err := git.CreateAnnotatedTag(finalVersion.String(), tagMessage); err != nil {
		return fmt.Errorf("failed to create tag: %w", err)
	}

	fmt.Printf("Tag %s created successfully.\n", finalVersion.String())
	fmt.Printf("Hint: run `git push origin %s` to share the tag.\n", finalVersion.String())
	return nil
}

func confirmTagCreation(tag string) (bool, error) {
	if tagAutoYes {
		fmt.Println("Auto-confirming tag creation (-y flag is set)")
		return true, nil
	}

	fmt.Printf("Create tag %s? [y/N]: ", tag)
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
