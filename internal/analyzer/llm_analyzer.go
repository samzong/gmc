package analyzer

import (
	"fmt"
	"strings"

	"github.com/samzong/gmc/internal/config"
	"github.com/samzong/gmc/internal/llm"
)

// LLMAnalyzer handles AI-powered analysis and suggestions
type LLMAnalyzer struct{}

// NewLLMAnalyzer creates a new LLM analyzer
func NewLLMAnalyzer() *LLMAnalyzer {
	return &LLMAnalyzer{}
}

// GenerateSuggestions generates improvement suggestions based on analysis results
func (l *LLMAnalyzer) GenerateSuggestions(result *AnalysisResult) error {
	cfg := config.GetConfig()

	// Check if LLM is available
	if cfg.APIKey == "" {
		// Graceful degradation: provide basic suggestions without LLM
		result.Suggestions = l.generateBasicSuggestions(result)
		return nil
	}

	// Generate AI-powered suggestions
	prompt := l.buildAnalysisPrompt(result, cfg.Role)

	suggestions, err := llm.GenerateCommitMessage(prompt, cfg.Model)
	if err != nil {
		// Fallback to basic suggestions if LLM fails
		result.Suggestions = l.generateBasicSuggestions(result)
		return nil // Don't return error, just use fallback
	}

	// Parse suggestions from LLM response
	result.Suggestions = l.parseSuggestions(suggestions)

	return nil
}

// buildAnalysisPrompt builds the prompt for LLM analysis
func (l *LLMAnalyzer) buildAnalysisPrompt(result *AnalysisResult, role string) string {
	var promptBuilder strings.Builder

	// Basic analysis info
	promptBuilder.WriteString(fmt.Sprintf("As a senior %s, please analyze the following commit history and provide improvement suggestions:\n\n", role))
	promptBuilder.WriteString(fmt.Sprintf("Total commits: %d\n", result.TotalCommits))
	promptBuilder.WriteString(fmt.Sprintf("Quality score: %.1f/100 (%s)\n\n", result.QualityScore, GetQualityLevel(result.QualityScore)))

	// Type distribution
	if len(result.TypeDistribution) > 0 {
		promptBuilder.WriteString("Commit type distribution:\n")
		for commitType, count := range result.TypeDistribution {
			promptBuilder.WriteString(fmt.Sprintf("- %s: %d commits\n", commitType, count))
		}
		promptBuilder.WriteString("\n")
	}

	// Poor commits examples
	if len(result.PoorCommits) > 0 {
		promptBuilder.WriteString("Commits that need improvement:\n")
		for _, commit := range result.PoorCommits {
			promptBuilder.WriteString(fmt.Sprintf("- %s: %s\n", commit.Hash, commit.Message))
		}
		promptBuilder.WriteString("\n")
	}

	// Team-specific info
	if result.IsTeamAnalysis && len(result.AuthorStats) > 0 {
		promptBuilder.WriteString("Team contributor status:\n")
		for author, stats := range result.AuthorStats {
			promptBuilder.WriteString(fmt.Sprintf("- %s: %d commits, quality score %.1f\n", author, stats.CommitCount, stats.QualityScore))
		}
		promptBuilder.WriteString("\n")
	}

	// Request specific suggestions
	if result.IsTeamAnalysis {
		promptBuilder.WriteString("Please provide:\n")
		promptBuilder.WriteString("1. 3 team commit quality improvement suggestions\n")
		promptBuilder.WriteString("2. Rewrite examples for problematic commits\n")
		promptBuilder.WriteString("3. Best practice suggestions for team collaboration\n\n")
	} else {
		promptBuilder.WriteString("Please provide:\n")
		promptBuilder.WriteString("1. 3 specific personal improvement suggestions\n")
		promptBuilder.WriteString("2. Rewrite examples for problematic commits\n")
		promptBuilder.WriteString("3. Best practices for commit messages\n\n")
	}

	promptBuilder.WriteString("Requirements: Each suggestion should be concise and practical, no more than 50 words.")

	return promptBuilder.String()
}

// parseSuggestions parses the LLM response into a list of suggestions
func (l *LLMAnalyzer) parseSuggestions(response string) []string {
	lines := strings.Split(response, "\n")
	suggestions := []string{}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Look for numbered suggestions or bullet points
		if strings.HasPrefix(line, "1.") || strings.HasPrefix(line, "2.") || strings.HasPrefix(line, "3.") ||
			strings.HasPrefix(line, "-") || strings.HasPrefix(line, "•") {
			// Clean up the suggestion
			suggestion := strings.TrimPrefix(line, "1.")
			suggestion = strings.TrimPrefix(suggestion, "2.")
			suggestion = strings.TrimPrefix(suggestion, "3.")
			suggestion = strings.TrimPrefix(suggestion, "-")
			suggestion = strings.TrimPrefix(suggestion, "•")
			suggestion = strings.TrimSpace(suggestion)

			if suggestion != "" && len(suggestion) > 10 { // Filter out too short suggestions
				suggestions = append(suggestions, suggestion)
			}
		}
	}

	// If we couldn't parse structured suggestions, try to split by sentences
	if len(suggestions) == 0 {
		sentences := strings.Split(response, "。")
		for _, sentence := range sentences {
			sentence = strings.TrimSpace(sentence)
			if len(sentence) > 20 && len(sentence) < 200 {
				suggestions = append(suggestions, sentence)
			}
		}
	}

	// Limit to 5 suggestions
	if len(suggestions) > 5 {
		suggestions = suggestions[:5]
	}

	return suggestions
}

// generateBasicSuggestions provides basic suggestions without LLM
func (l *LLMAnalyzer) generateBasicSuggestions(result *AnalysisResult) []string {
	suggestions := []string{}

	// Quality-based suggestions
	if result.QualityScore < 60 {
		suggestions = append(suggestions, "Use Conventional Commits format: type(scope): description")
		suggestions = append(suggestions, "Keep commit message length between 50-72 characters")
		suggestions = append(suggestions, "Start with specific verbs, avoid vague words like 'update', 'fix stuff'")
	} else if result.QualityScore < 80 {
		suggestions = append(suggestions, "Continue good commit habits, be more specific in scope section")
		suggestions = append(suggestions, "Consider including more context information in commit messages")
	}

	// Type distribution based suggestions
	if len(result.TypeDistribution) > 0 {
		totalCommits := result.TotalCommits

		// Check for missing test commits
		testCommits := result.TypeDistribution["test"]
		if float64(testCommits)/float64(totalCommits) < 0.1 {
			suggestions = append(suggestions, "Consider adding more test-related commits to improve code quality")
		}

		// Check for too many chore commits
		choreCommits := result.TypeDistribution["chore"]
		if float64(choreCommits)/float64(totalCommits) > 0.3 {
			suggestions = append(suggestions, "Reduce miscellaneous commits, try to merge related changes into feature commits")
		}

		// Check for documentation
		docsCommits := result.TypeDistribution["docs"]
		if docsCommits == 0 && totalCommits > 10 {
			suggestions = append(suggestions, "Consider adding documentation-related commits to improve project maintainability")
		}
	}

	// Team-specific suggestions
	if result.IsTeamAnalysis {
		suggestions = append(suggestions, "Recommend team to standardize commit message format, use code review to ensure quality")

		// Check author distribution
		if len(result.AuthorStats) > 1 {
			suggestions = append(suggestions, "Encourage team members to learn from each other's excellent commit practices")
		}
	}

	// Limit to 5 suggestions
	if len(suggestions) > 5 {
		suggestions = suggestions[:5]
	}

	// Ensure we have at least some suggestions
	if len(suggestions) == 0 {
		suggestions = append(suggestions, "Continue maintaining good commit habits")
		suggestions = append(suggestions, "Regularly review commit history for continuous improvement")
	}

	return suggestions
}
