package version

import (
	"testing"

	"github.com/samzong/gmc/internal/git"
	"github.com/stretchr/testify/assert"
)

func TestParseSemVer(t *testing.T) {
	v, err := ParseSemVer("v1.2.3")
	assert.NoError(t, err)
	assert.Equal(t, 1, v.Major)
	assert.Equal(t, 2, v.Minor)
	assert.Equal(t, 3, v.Patch)

	v2, err := ParseSemVer("0.0.0")
	assert.NoError(t, err)
	assert.True(t, v2.Equal(SemVer{}))
}

func TestParseSemVerInvalid(t *testing.T) {
	_, err := ParseSemVer("invalid")
	assert.Error(t, err)

	_, err = ParseSemVer("1.2")
	assert.Error(t, err)
}

func TestSemVerComparison(t *testing.T) {
	base := SemVer{Major: 1, Minor: 2, Patch: 3}
	assert.Equal(t, "v1.2.3", base.String())
	assert.True(t, base.LessThan(SemVer{Major: 2}))
	assert.True(t, base.GreaterThan(SemVer{Major: 1, Minor: 2, Patch: 2}))
}

func TestSuggestWithRulesMajor(t *testing.T) {
	base, _ := ParseSemVer("v1.2.3")
	commits := []git.CommitInfo{
		{Message: "feat!: new API"},
		{Message: "fix: adjust tests"},
	}

	result := SuggestWithRules(base, commits)

	assert.Equal(t, BumpMajor, result.BumpType)
	assert.Equal(t, "v2.0.0", result.NextVersion.String())
	assert.Contains(t, result.Reason, "breaking change")
}

func TestSuggestWithRulesBreakingInBody(t *testing.T) {
	base, _ := ParseSemVer("v0.1.4")
	commits := []git.CommitInfo{
		{Message: "chore: update deps", Body: "BREAKING CHANGE: new config"},
	}

	result := SuggestWithRules(base, commits)

	assert.Equal(t, BumpMajor, result.BumpType)
	assert.Equal(t, "v1.0.0", result.NextVersion.String())
}

func TestSuggestWithRulesMinor(t *testing.T) {
	base, _ := ParseSemVer("v0.1.4")
	commits := []git.CommitInfo{
		{Message: "feat: add CLI"},
		{Message: "docs: update readme"},
	}

	result := SuggestWithRules(base, commits)

	assert.Equal(t, BumpMinor, result.BumpType)
	assert.Equal(t, "v0.2.0", result.NextVersion.String())
	assert.Contains(t, result.Reason, "feature")
}

func TestSuggestWithRulesPatch(t *testing.T) {
	base, _ := ParseSemVer("v0.1.4")
	commits := []git.CommitInfo{
		{Message: "fix: resolve bug"},
		{Message: "chore: update"},
	}

	result := SuggestWithRules(base, commits)

	assert.Equal(t, BumpPatch, result.BumpType)
	assert.Equal(t, "v0.1.5", result.NextVersion.String())
}

func TestSuggestWithRulesNoChange(t *testing.T) {
	base, _ := ParseSemVer("v0.1.4")
	commits := []git.CommitInfo{
		{Message: "docs: update readme"},
		{Message: "chore: tidy"},
	}

	result := SuggestWithRules(base, commits)

	assert.Equal(t, BumpNone, result.BumpType)
	assert.True(t, result.NextVersion.Equal(base))
	assert.Contains(t, result.Reason, "documentation")
}

func TestSuggestWithRulesNoCommits(t *testing.T) {
	base, _ := ParseSemVer("v0.1.4")

	result := SuggestWithRules(base, nil)

	assert.Equal(t, BumpNone, result.BumpType)
	assert.True(t, result.NextVersion.Equal(base))
	assert.Contains(t, result.Reason, "No commits")
}
