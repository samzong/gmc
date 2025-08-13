package analyzer

import (
	"regexp"
	"strings"

	"github.com/samzong/gmc/internal/git"
)

// AnalyzerImpl implements the Analyzer interface
type AnalyzerImpl struct {
	qualityEvaluator QualityEvaluator
}

// NewAnalyzer creates a new analyzer instance
func NewAnalyzer() Analyzer {
	return &AnalyzerImpl{
		qualityEvaluator: NewQualityEvaluator(),
	}
}

// Analyze performs the analysis based on the mode
func (a *AnalyzerImpl) Analyze(teamMode bool) (*AnalysisResult, error) {
	// Determine commit limit based on mode
	limit := 50 // Personal mode
	if teamMode {
		limit = 200 // Team mode
	}

	// Get commit history
	commits, err := git.GetCommitHistory(limit, teamMode)
	if err != nil {
		return nil, err
	}

	if len(commits) == 0 {
		return &AnalysisResult{
			TotalCommits:     0,
			QualityScore:     0,
			TypeDistribution: make(map[string]int),
			Suggestions:      []string{"No commits found in the repository."},
			PoorCommits:      []git.CommitInfo{},
			IsTeamAnalysis:   teamMode,
			AuthorStats:      make(map[string]AuthorStats),
		}, nil
	}

	// Evaluate commit quality
	metrics := a.qualityEvaluator.EvaluateBatch(commits)

	// Calculate overall quality score
	totalScore := 0.0
	for _, metric := range metrics {
		totalScore += metric.OverallScore
	}
	averageScore := totalScore / float64(len(metrics))

	// Analyze commit type distribution
	typeDistribution := a.analyzeTypeDistribution(commits)

	// Identify poor quality commits
	poorCommits := a.identifyPoorCommits(commits, metrics)

	result := &AnalysisResult{
		TotalCommits:     len(commits),
		QualityScore:     averageScore,
		TypeDistribution: typeDistribution,
		Suggestions:      []string{}, // Will be filled by LLM analyzer
		PoorCommits:      poorCommits,
		IsTeamAnalysis:   teamMode,
	}

	// Add team-specific analysis
	if teamMode {
		result.AuthorStats = a.analyzeAuthorStats(commits, metrics)
	}

	return result, nil
}

// analyzeTypeDistribution analyzes the distribution of commit types
func (a *AnalyzerImpl) analyzeTypeDistribution(commits []git.CommitInfo) map[string]int {
	distribution := make(map[string]int)

	// Conventional commit type pattern
	typePattern := regexp.MustCompile(`^(feat|fix|docs|style|refactor|perf|test|chore|build|ci)(\([^)]+\))?:`)

	for _, commit := range commits {
		matches := typePattern.FindStringSubmatch(commit.Message)
		if len(matches) > 1 {
			commitType := matches[1]
			distribution[commitType]++
		} else {
			// Try to infer type from message content
			inferredType := a.inferCommitType(commit.Message)
			distribution[inferredType]++
		}
	}

	return distribution
}

// inferCommitType tries to infer commit type from message content
func (a *AnalyzerImpl) inferCommitType(message string) string {
	message = strings.ToLower(message)

	if strings.Contains(message, "fix") || strings.Contains(message, "bug") {
		return "fix"
	}
	if strings.Contains(message, "add") || strings.Contains(message, "feature") || strings.Contains(message, "implement") {
		return "feat"
	}
	if strings.Contains(message, "doc") || strings.Contains(message, "readme") {
		return "docs"
	}
	if strings.Contains(message, "test") {
		return "test"
	}
	if strings.Contains(message, "refactor") {
		return "refactor"
	}
	if strings.Contains(message, "style") || strings.Contains(message, "format") {
		return "style"
	}
	if strings.Contains(message, "perf") || strings.Contains(message, "performance") {
		return "perf"
	}

	return "chore"
}

// identifyPoorCommits identifies commits with low quality scores
func (a *AnalyzerImpl) identifyPoorCommits(commits []git.CommitInfo, metrics []QualityMetrics) []git.CommitInfo {
	var poorCommits []git.CommitInfo

	for i, metric := range metrics {
		if metric.OverallScore < 40 { // Threshold for poor quality
			poorCommits = append(poorCommits, commits[i])
		}
	}

	// Limit to top 5 worst commits
	if len(poorCommits) > 5 {
		poorCommits = poorCommits[:5]
	}

	return poorCommits
}

// analyzeAuthorStats analyzes statistics for each author in team mode
func (a *AnalyzerImpl) analyzeAuthorStats(commits []git.CommitInfo, metrics []QualityMetrics) map[string]AuthorStats {
	authorStats := make(map[string]AuthorStats)
	authorCommits := make(map[string][]int) // author -> commit indices

	// Group commits by author
	for i, commit := range commits {
		author := commit.Author
		if _, exists := authorCommits[author]; !exists {
			authorCommits[author] = []int{}
		}
		authorCommits[author] = append(authorCommits[author], i)
	}

	// Calculate stats for each author
	for author, commitIndices := range authorCommits {
		totalScore := 0.0
		issues := []string{}

		for _, idx := range commitIndices {
			metric := metrics[idx]
			totalScore += metric.OverallScore

			// Identify common issues
			if metric.ConventionalScore < 20 {
				issues = append(issues, "Does not follow Conventional Commits format")
			}
			if metric.LengthScore < 15 {
				issues = append(issues, "Inappropriate commit message length")
			}
			if metric.ClarityScore < 15 {
				issues = append(issues, "Commit message lacks clarity")
			}
		}

		averageScore := totalScore / float64(len(commitIndices))

		// Get unique issues (top 3)
		uniqueIssues := a.getUniqueIssues(issues, 3)

		authorStats[author] = AuthorStats{
			Name:         author,
			CommitCount:  len(commitIndices),
			QualityScore: averageScore,
			TopIssues:    uniqueIssues,
		}
	}

	return authorStats
}

// getUniqueIssues returns unique issues, limited to maxCount
func (a *AnalyzerImpl) getUniqueIssues(issues []string, maxCount int) []string {
	seen := make(map[string]bool)
	unique := []string{}

	for _, issue := range issues {
		if !seen[issue] && len(unique) < maxCount {
			seen[issue] = true
			unique = append(unique, issue)
		}
	}

	return unique
}
