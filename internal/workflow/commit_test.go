package workflow

import (
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/samzong/gmc/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeGit struct {
	diff    string
	stats   string
	files   []string
	commits []string
}

func (f *fakeGit) IsGitRepository() bool     { return true }
func (f *fakeGit) CheckGitRepository() error { return nil }
func (f *fakeGit) AddAll() error             { return nil }
func (f *fakeGit) StageFiles([]string) error { return nil }

func (f *fakeGit) GetStagedDiff() (string, error)        { return f.diff, nil }
func (f *fakeGit) GetStagedDiffStats() (string, error)   { return f.stats, nil }
func (f *fakeGit) GetFilesDiff([]string) (string, error) { return f.diff, nil }
func (f *fakeGit) ParseStagedFiles() ([]string, error)   { return f.files, nil }

func (f *fakeGit) ResolveFiles(paths []string) ([]string, error) { return paths, nil }

func (f *fakeGit) CheckFileStatus([]string) ([]string, []string, []string, error) {
	return f.files, nil, nil, nil
}

func (f *fakeGit) Commit(message string, _ ...string) error {
	f.commits = append(f.commits, message)
	return nil
}

func (f *fakeGit) CommitFiles(message string, _ []string, _ ...string) error {
	f.commits = append(f.commits, message)
	return nil
}

func (f *fakeGit) CreateAndSwitchBranch(string) error { return nil }

type fakeLLM struct {
	message string
	prompts []string
}

func (f *fakeLLM) GenerateCommitMessage(prompt, _ string) (string, error) {
	f.prompts = append(f.prompts, prompt)
	return f.message, nil
}

type autoPrompter struct{}

func (autoPrompter) GetConfirmation(string, bool) (Action, string, error) {
	return ActionCommit, "", nil
}

func runFlow(t *testing.T, llmMessage string, opts CommitOptions) (*fakeGit, *fakeLLM, error) {
	t.Helper()

	git := &fakeGit{
		diff:  "diff --git a/a.txt b/a.txt\n@@ -0,0 +1 @@\n+hello\n",
		stats: "1\t0\ta.txt",
		files: []string{"a.txt"},
	}
	llm := &fakeLLM{message: llmMessage}

	opts.ErrWriter = io.Discard
	opts.OutWriter = io.Discard

	flow := NewCommitFlow(git, llm, &config.Config{}, opts)
	flow.SetPrompter(autoPrompter{})

	return git, llm, flow.Run(nil)
}

func TestCommitFlowUnwrapsFencedMessage(t *testing.T) {
	git, _, err := runFlow(t, "```\nfeat: add thing\n```", CommitOptions{})
	require.NoError(t, err)
	assert.Equal(t, []string{"feat: add thing"}, git.commits)
}

func TestCommitFlowStripsPreamble(t *testing.T) {
	git, _, err := runFlow(t, "Here is the commit message:\nfeat: add thing", CommitOptions{})
	require.NoError(t, err)
	assert.Equal(t, []string{"feat: add thing"}, git.commits)
}

func TestCommitFlowAppliesIssueSuffix(t *testing.T) {
	git, _, err := runFlow(t, "feat: add thing", CommitOptions{IssueNum: "123"})
	require.NoError(t, err)
	assert.Equal(t, []string{"feat: add thing (#123)"}, git.commits)
}

func TestCommitFlowRejectsMessageWithoutSubject(t *testing.T) {
	for _, message := range []string{"", "   ", "```", "```\n```", "Here is the commit message:"} {
		t.Run(strconv.Quote(message), func(t *testing.T) {
			git, _, err := runFlow(t, message, CommitOptions{IssueNum: "123"})
			require.Error(t, err)
			assert.Empty(t, git.commits, "nothing may be committed")
		})
	}
}

func TestCommitFlowSendsDiffAndStatsToThePrompt(t *testing.T) {
	_, llm, err := runFlow(t, "feat: add thing", CommitOptions{})
	require.NoError(t, err)

	require.Len(t, llm.prompts, 1)
	assert.Contains(t, llm.prompts[0], "+hello")
	assert.Contains(t, llm.prompts[0], "a.txt")
}

func bigDiff() (diff, stats string) {
	var b strings.Builder
	var numstat strings.Builder
	for _, name := range []string{"main.go", "go.sum"} {
		b.WriteString("diff --git a/" + name + " b/" + name + "\n")
		b.WriteString("@@ -1,40 +1,40 @@\n")
		for range 40 {
			b.WriteString("-old line that is long enough to add up quickly\n")
			b.WriteString("+new line that is long enough to add up quickly\n")
		}
		numstat.WriteString("40\t40\t" + name + "\n")
	}
	return b.String(), numstat.String()
}

func TestCommitFlowPassesStatsThrough(t *testing.T) {
	diff, stats := bigDiff()
	require.Greater(t, len(diff), 4000, "the fixture must exceed the prompt budget")

	git := &fakeGit{diff: diff, stats: stats, files: []string{"main.go", "go.sum"}}
	llm := &fakeLLM{message: "chore: bump deps"}
	flow := NewCommitFlow(git, llm, &config.Config{},
		CommitOptions{ErrWriter: io.Discard, OutWriter: io.Discard})
	flow.SetPrompter(autoPrompter{})

	require.NoError(t, flow.Run(nil))
	require.Len(t, llm.prompts, 1)

	assert.Contains(t, llm.prompts[0], "(+40/-40)")
	assert.NotContains(t, llm.prompts[0], "content is too long, truncated",
		"a diff with stats must not fall back to the naive cut")
}
