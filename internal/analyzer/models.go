package analyzer

import "github.com/samzong/gmc/internal/git"

// AnalysisResult represents the complete analysis result
type AnalysisResult struct {
	TotalCommits     int              `json:"total_commits"`
	QualityScore     float64          `json:"quality_score"`     // 0-100
	TypeDistribution map[string]int   `json:"type_distribution"` // feat: 5, fix: 3, etc.
	Suggestions      []string         `json:"suggestions"`
	PoorCommits      []git.CommitInfo `json:"poor_commits"` // 质量差的提交示例

	// 团队分析相关
	IsTeamAnalysis bool                   `json:"is_team_analysis"`
	AuthorStats    map[string]AuthorStats `json:"author_stats,omitempty"`
}

// AuthorStats represents statistics for a single author
type AuthorStats struct {
	Name         string   `json:"name"`
	CommitCount  int      `json:"commit_count"`
	QualityScore float64  `json:"quality_score"`
	TopIssues    []string `json:"top_issues"` // 主要问题
}

// QualityMetrics represents quality metrics for a single commit
type QualityMetrics struct {
	ConventionalScore float64 `json:"conventional_score"` // 0-40
	LengthScore       float64 `json:"length_score"`       // 0-30
	ClarityScore      float64 `json:"clarity_score"`      // 0-30
	OverallScore      float64 `json:"overall_score"`      // 0-100
}

// Analyzer defines the core analyzer interface
type Analyzer interface {
	// Analyze performs the analysis based on the mode
	Analyze(teamMode bool) (*AnalysisResult, error)
}

// QualityEvaluator defines the quality evaluation interface
type QualityEvaluator interface {
	// EvaluateCommit evaluates the quality of a single commit
	EvaluateCommit(commit git.CommitInfo) QualityMetrics

	// EvaluateBatch evaluates multiple commits
	EvaluateBatch(commits []git.CommitInfo) []QualityMetrics
}
