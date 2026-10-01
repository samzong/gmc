package workflow

import (
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/samzong/gmc/internal/branch"
	"github.com/samzong/gmc/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type commitCall struct {
	message   string
	files     []string
	args      []string
	selective bool
}

type fakeGit struct {
	diff      string
	stats     string
	files     []string
	staged    []string
	modified  []string
	untracked []string

	calls       []string
	stagedFiles [][]string
	commits     []commitCall
	branches    []string
}

func (f *fakeGit) AddAll() error {
	f.calls = append(f.calls, "add-all")
	return nil
}

func (f *fakeGit) StageFiles(files []string) error {
	f.calls = append(f.calls, "stage")
	f.stagedFiles = append(f.stagedFiles, files)
	return nil
}

func (f *fakeGit) GetStagedDiff() (string, error)        { return f.diff, nil }
func (f *fakeGit) GetStagedDiffStats() (string, error)   { return f.stats, nil }
func (f *fakeGit) GetFilesDiff([]string) (string, error) { return f.diff, nil }
func (f *fakeGit) ParseStagedFiles() ([]string, error)   { return f.files, nil }

func (f *fakeGit) ResolveFiles(paths []string) ([]string, error) { return paths, nil }

func (f *fakeGit) CheckFileStatus([]string) ([]string, []string, []string, error) {
	return f.staged, f.modified, f.untracked, nil
}

func (f *fakeGit) Commit(message string, args ...string) error {
	f.calls = append(f.calls, "commit")
	f.commits = append(f.commits, commitCall{message: message, args: args})
	return nil
}

func (f *fakeGit) CommitFiles(message string, files []string, args ...string) error {
	f.calls = append(f.calls, "commit-files")
	f.commits = append(f.commits, commitCall{message: message, files: files, args: args, selective: true})
	return nil
}

func (f *fakeGit) CreateAndSwitchBranch(name string) error {
	f.calls = append(f.calls, "branch")
	f.branches = append(f.branches, name)
	return nil
}

func (f *fakeGit) messages() []string {
	var messages []string
	for _, c := range f.commits {
		messages = append(messages, c.message)
	}
	return messages
}

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
	assert.Equal(t, []string{"feat: add thing"}, git.messages())
}

func TestCommitFlowStripsPreamble(t *testing.T) {
	git, _, err := runFlow(t, "Here is the commit message:\nfeat: add thing", CommitOptions{})
	require.NoError(t, err)
	assert.Equal(t, []string{"feat: add thing"}, git.messages())
}

func TestCommitFlowAppliesIssueSuffix(t *testing.T) {
	git, _, err := runFlow(t, "feat: add thing", CommitOptions{IssueNum: "123"})
	require.NoError(t, err)
	assert.Equal(t, []string{"feat: add thing (#123)"}, git.messages())
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

const selectiveDiff = "diff --git a/m.go b/m.go\n@@ -1 +1 @@\n-old\n+new\n"

func runSelective(t *testing.T, git *fakeGit, opts CommitOptions, fileArgs []string) (*fakeLLM, error) {
	t.Helper()
	llm := &fakeLLM{message: "feat: change files"}
	opts.ErrWriter = io.Discard
	opts.OutWriter = io.Discard
	flow := NewCommitFlow(git, llm, &config.Config{}, opts)
	flow.SetPrompter(autoPrompter{})
	return llm, flow.Run(fileArgs)
}

func TestSelectiveCommitWithoutAddAllRequiresStagedFiles(t *testing.T) {
	git := &fakeGit{diff: selectiveDiff, modified: []string{"m.go"}, untracked: []string{"u.go"}}

	llm, err := runSelective(t, git, CommitOptions{}, []string{"m.go", "u.go"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "gmc -a m.go u.go")
	assert.Empty(t, git.stagedFiles)
	assert.Empty(t, git.commits)
	assert.Empty(t, llm.prompts)
}

func TestSelectiveCommitWithAddAllStagesAndCommitsFiles(t *testing.T) {
	tests := []struct {
		name          string
		opts          CommitOptions
		staged        []string
		modified      []string
		untracked     []string
		wantStaged    [][]string
		wantCommitted []string
		wantArgs      []string
		absentArgs    []string
	}{
		{
			name:          "stages modified and untracked with signoff",
			opts:          CommitOptions{AddAll: true},
			staged:        []string{"s.go"},
			modified:      []string{"m.go"},
			untracked:     []string{"u.go"},
			wantStaged:    [][]string{{"m.go", "u.go"}},
			wantCommitted: []string{"s.go", "m.go", "u.go"},
			wantArgs:      []string{"-s"},
			absentArgs:    []string{"--no-verify"},
		},
		{
			name:          "no signoff",
			opts:          CommitOptions{AddAll: true, NoSignoff: true},
			modified:      []string{"m.go"},
			untracked:     []string{"u.go"},
			wantStaged:    [][]string{{"m.go", "u.go"}},
			wantCommitted: []string{"m.go", "u.go"},
			absentArgs:    []string{"-s"},
		},
		{
			name:          "no verify",
			opts:          CommitOptions{AddAll: true, NoVerify: true},
			modified:      []string{"m.go"},
			wantStaged:    [][]string{{"m.go"}},
			wantCommitted: []string{"m.go"},
			wantArgs:      []string{"--no-verify", "-s"},
		},
		{
			name:          "already staged only",
			opts:          CommitOptions{AddAll: true},
			staged:        []string{"s.go"},
			wantCommitted: []string{"s.go"},
			wantArgs:      []string{"-s"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			git := &fakeGit{diff: selectiveDiff, staged: tt.staged, modified: tt.modified, untracked: tt.untracked}

			llm, err := runSelective(t, git, tt.opts, []string{"s.go", "m.go", "u.go"})

			require.NoError(t, err)
			assert.Len(t, llm.prompts, 1)
			for i, want := range tt.wantStaged {
				require.Greater(t, len(git.stagedFiles), i)
				assert.ElementsMatch(t, want, git.stagedFiles[i])
			}
			assert.Len(t, git.stagedFiles, len(tt.wantStaged))
			require.Len(t, git.commits, 1)
			commit := git.commits[0]
			assert.True(t, commit.selective)
			assert.Equal(t, "feat: change files", commit.message)
			assert.ElementsMatch(t, tt.wantCommitted, commit.files)
			for _, arg := range tt.wantArgs {
				assert.Contains(t, commit.args, arg)
			}
			for _, arg := range tt.absentArgs {
				assert.NotContains(t, commit.args, arg)
			}
		})
	}
}

func TestSelectiveCommitWithAddAllAndNoChanges(t *testing.T) {
	git := &fakeGit{diff: selectiveDiff}

	llm, err := runSelective(t, git, CommitOptions{AddAll: true}, []string{"m.go"})

	require.Error(t, err)
	assert.Empty(t, git.stagedFiles)
	assert.Empty(t, git.commits)
	assert.Empty(t, llm.prompts)
}

func TestSelectiveCommitDryRunDoesNotCommit(t *testing.T) {
	git := &fakeGit{diff: selectiveDiff, staged: []string{"m.go"}}

	llm, err := runSelective(t, git, CommitOptions{DryRun: true}, []string{"m.go"})

	require.NoError(t, err)
	assert.Len(t, llm.prompts, 1)
	assert.Empty(t, git.commits)
}

func TestBranchDescCreatesBranchBeforeCommit(t *testing.T) {
	git := &fakeGit{diff: selectiveDiff, staged: []string{"m.go"}}

	_, err := runSelective(t, git, CommitOptions{BranchDesc: "add login page"}, []string{"m.go"})

	require.NoError(t, err)
	assert.Equal(t, []string{branch.GenerateName("add login page")}, git.branches)
	assert.Equal(t, []string{"branch", "commit-files"}, git.calls)
}

func TestBranchDescWithoutUsableNameFails(t *testing.T) {
	require.Empty(t, branch.GenerateName("!!!"))
	git := &fakeGit{diff: selectiveDiff, staged: []string{"m.go"}}

	llm, err := runSelective(t, git, CommitOptions{BranchDesc: "!!!"}, []string{"m.go"})

	require.Error(t, err)
	assert.Empty(t, git.branches)
	assert.Empty(t, git.commits)
	assert.Empty(t, llm.prompts)
}

func TestAddAllWithoutFileArgsStagesEverythingThenCommits(t *testing.T) {
	git := &fakeGit{diff: selectiveDiff, stats: "1\t1\tm.go", files: []string{"m.go"}}

	_, err := runSelective(t, git, CommitOptions{AddAll: true}, nil)

	require.NoError(t, err)
	assert.Equal(t, []string{"add-all", "commit"}, git.calls)
	require.Len(t, git.commits, 1)
	assert.False(t, git.commits[0].selective)
	assert.Contains(t, git.commits[0].args, "-s")
}
