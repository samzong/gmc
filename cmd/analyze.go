package cmd

import (
	"fmt"
	"os"

	"github.com/samzong/gmc/internal/analyzer"
	"github.com/samzong/gmc/internal/git"
	"github.com/spf13/cobra"
)

var (
	teamMode bool

	analyzeCmd = &cobra.Command{
		Use:   "analyze",
		Short: "Analyze commit history and provide quality insights",
		Long: `Analyze commit history to provide quality assessment and improvement suggestions.

Examples:
  gmc analyze           # Analyze personal commits (last 50)
  gmc analyze --team    # Analyze team commits (last 200)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAnalyze()
		},
	}
)

func init() {
	analyzeCmd.Flags().BoolVar(&teamMode, "team", false, "Analyze team commits instead of personal commits")
	rootCmd.AddCommand(analyzeCmd)
}

func runAnalyze() error {
	// Check if we're in a git repository
	if err := git.CheckGitRepository(); err != nil {
		return fmt.Errorf("analysis failed: %w", err)
	}

	// Create analyzer and visualizer
	analysisEngine := analyzer.NewAnalyzer()
	visualizer := analyzer.NewVisualizer()
	llmAnalyzer := analyzer.NewLLMAnalyzer()

	// Perform analysis
	fmt.Println("Analyzing commit history...")
	result, err := analysisEngine.Analyze(teamMode)
	if err != nil {
		return fmt.Errorf("analysis failed: %w", err)
	}

	// Generate AI suggestions
	if err := llmAnalyzer.GenerateSuggestions(result); err != nil {
		// Non-fatal error, continue with basic suggestions
		fmt.Fprintf(os.Stderr, "Warning: Failed to generate AI suggestions: %v\n", err)
	}

	// Display results
	displayAnalysisResults(result, visualizer)

	return nil
}

func displayAnalysisResults(result *analyzer.AnalysisResult, visualizer *analyzer.Visualizer) {
	// Clear screen and show header
	fmt.Print("\n")
	fmt.Print(visualizer.GenerateSummaryHeader(result))
	fmt.Print("\n")

	// Show type distribution chart
	if len(result.TypeDistribution) > 0 {
		fmt.Print(visualizer.GenerateTypeDistributionChart(result.TypeDistribution))
		fmt.Print("\n")
	}

	// Show team-specific information
	if result.IsTeamAnalysis && len(result.AuthorStats) > 0 {
		fmt.Print(visualizer.GenerateAuthorRankingChart(result.AuthorStats))
		fmt.Print("\n")
	}

	// Show suggestions
	if len(result.Suggestions) > 0 {
		fmt.Print(visualizer.GenerateSuggestionsSection(result.Suggestions, result.IsTeamAnalysis))
		fmt.Print("\n")
	}

	// Show poor commits examples
	if len(result.PoorCommits) > 0 {
		fmt.Print(visualizer.GeneratePoorCommitsSection(result.PoorCommits))
	}

	// Show summary message
	if result.TotalCommits == 0 {
		fmt.Println("💡 Tip: No commits found in the current repository.")
		if !result.IsTeamAnalysis {
			fmt.Println("   Please ensure your git username is configured correctly: git config user.name")
		}
	} else {
		qualityLevel := analyzer.GetQualityLevel(result.QualityScore)
		switch qualityLevel {
		case "Excellent":
			fmt.Println("🎉 Congratulations! Your commit quality is excellent, keep it up!")
		case "Good":
			fmt.Println("👍 Your commit quality is good, there's room for improvement!")
		default:
			fmt.Println("💪 Commit quality needs improvement, please refer to the suggestions above!")
		}
	}
}
