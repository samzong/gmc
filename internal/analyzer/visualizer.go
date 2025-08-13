package analyzer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/samzong/gmc/internal/git"
)

// Visualizer handles ASCII chart generation
type Visualizer struct{}

// NewVisualizer creates a new visualizer
func NewVisualizer() *Visualizer {
	return &Visualizer{}
}

// GenerateTypeDistributionChart generates an ASCII bar chart for commit type distribution
func (v *Visualizer) GenerateTypeDistributionChart(distribution map[string]int) string {
	if len(distribution) == 0 {
		return "No commit types to display"
	}

	// Sort types by count (descending)
	type typeCount struct {
		Type  string
		Count int
	}

	var types []typeCount
	maxCount := 0
	for t, count := range distribution {
		types = append(types, typeCount{Type: t, Count: count})
		if count > maxCount {
			maxCount = count
		}
	}

	sort.Slice(types, func(i, j int) bool {
		return types[i].Count > types[j].Count
	})

	// Generate chart
	var chart strings.Builder
	chart.WriteString("📈 Commit Type Distribution:\n")

	// Calculate bar width (max 20 characters)
	maxBarWidth := 20

	for _, tc := range types {
		// Calculate bar length
		barLength := 0
		if maxCount > 0 {
			barLength = (tc.Count * maxBarWidth) / maxCount
			if barLength == 0 && tc.Count > 0 {
				barLength = 1 // Ensure at least 1 character for non-zero counts
			}
		}

		// Create bar
		bar := strings.Repeat("█", barLength)

		// Format line: "type: ████████ count"
		chart.WriteString(fmt.Sprintf("%-8s %s %d\n", tc.Type+":", bar, tc.Count))
	}

	return chart.String()
}

// GenerateAuthorRankingChart generates an ASCII chart for author rankings
func (v *Visualizer) GenerateAuthorRankingChart(authorStats map[string]AuthorStats) string {
	if len(authorStats) == 0 {
		return "No authors to display"
	}

	// Sort authors by quality score (descending)
	type authorRank struct {
		Name         string
		CommitCount  int
		QualityScore float64
	}

	var authors []authorRank
	maxScore := 0.0
	for _, stats := range authorStats {
		authors = append(authors, authorRank{
			Name:         stats.Name,
			CommitCount:  stats.CommitCount,
			QualityScore: stats.QualityScore,
		})
		if stats.QualityScore > maxScore {
			maxScore = stats.QualityScore
		}
	}

	sort.Slice(authors, func(i, j int) bool {
		return authors[i].QualityScore > authors[j].QualityScore
	})

	// Generate chart
	var chart strings.Builder
	chart.WriteString("👥 Contributor Ranking:\n")

	// Calculate bar width (max 15 characters for score visualization)
	maxBarWidth := 15

	for _, author := range authors {
		// Calculate bar length based on quality score
		barLength := 0
		if maxScore > 0 {
			barLength = int((author.QualityScore * float64(maxBarWidth)) / maxScore)
			if barLength == 0 && author.QualityScore > 0 {
				barLength = 1
			}
		}

		// Create bar
		bar := strings.Repeat("█", barLength)

		// Truncate long names
		displayName := author.Name
		if len(displayName) > 12 {
			displayName = displayName[:9] + "..."
		}

		// Format line: "Name     | ████████████ score (count commits)"
		chart.WriteString(fmt.Sprintf("%-12s | %s %.0f pts (%d commits)\n",
			displayName, bar, author.QualityScore, author.CommitCount))
	}

	return chart.String()
}

// GenerateQualityIndicator generates a visual quality indicator
func (v *Visualizer) GenerateQualityIndicator(score float64) string {
	var indicator strings.Builder

	// Quality level (used in color determination)

	// Visual indicator based on score
	var emoji string
	var color string

	switch {
	case score >= 80:
		emoji = "🟢"
		color = "Excellent"
	case score >= 60:
		emoji = "🟡"
		color = "Good"
	default:
		emoji = "🔴"
		color = "Needs Improvement"
	}

	indicator.WriteString(fmt.Sprintf("%s %.1f/100 (%s)", emoji, score, color))

	return indicator.String()
}

// GenerateSummaryHeader generates a formatted header for the analysis summary
func (v *Visualizer) GenerateSummaryHeader(result *AnalysisResult) string {
	var header strings.Builder

	if result.IsTeamAnalysis {
		header.WriteString("📊 Team Commit Quality Analysis")
		if result.TotalCommits > 0 {
			header.WriteString(fmt.Sprintf(" (last %d commits)", result.TotalCommits))
		}
	} else {
		header.WriteString("📊 Personal Commit Quality Analysis")
	}

	header.WriteString("\n\n")
	header.WriteString(fmt.Sprintf("Total Commits: %d\n", result.TotalCommits))
	header.WriteString(fmt.Sprintf("Quality Score: %s\n", v.GenerateQualityIndicator(result.QualityScore)))

	return header.String()
}

// GenerateSuggestionsSection generates a formatted suggestions section
func (v *Visualizer) GenerateSuggestionsSection(suggestions []string, isTeamAnalysis bool) string {
	if len(suggestions) == 0 {
		return ""
	}

	var section strings.Builder

	if isTeamAnalysis {
		section.WriteString("🎯 Team Improvement Suggestions:\n")
	} else {
		section.WriteString("🤖 AI Improvement Suggestions:\n")
	}

	for i, suggestion := range suggestions {
		section.WriteString(fmt.Sprintf("%d. %s\n", i+1, suggestion))
	}

	return section.String()
}

// GeneratePoorCommitsSection generates a section showing poor quality commits with examples
func (v *Visualizer) GeneratePoorCommitsSection(poorCommits []git.CommitInfo) string {
	if len(poorCommits) == 0 {
		return ""
	}

	var section strings.Builder
	section.WriteString("\nCommits that need improvement:\n")

	for _, commit := range poorCommits {
		// Suggest a better version
		betterMessage := v.suggestBetterCommitMessage(commit.Message)
		section.WriteString(fmt.Sprintf("- %s: %s → %s\n",
			commit.Hash, commit.Message, betterMessage))
	}

	return section.String()
}

// suggestBetterCommitMessage suggests an improved version of a commit message
func (v *Visualizer) suggestBetterCommitMessage(message string) string {
	message = strings.TrimSpace(message)
	lowerMessage := strings.ToLower(message)

	// Simple heuristics for improvement suggestions
	if strings.Contains(lowerMessage, "fix") {
		return "fix(component): resolve specific issue description"
	}
	if strings.Contains(lowerMessage, "add") {
		return "feat(component): implement specific feature"
	}
	if strings.Contains(lowerMessage, "update") {
		return "feat(component): enhance specific functionality"
	}
	if strings.Contains(lowerMessage, "change") {
		return "refactor(component): improve specific implementation"
	}

	// Default suggestion
	return "type(scope): clear description of what was changed"
}
