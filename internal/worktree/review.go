package worktree

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	reviewProviderGitHub = "github"
	reviewProviderGitLab = "gitlab"
	reviewCacheTTL       = 5 * time.Minute
	reviewCacheVersion   = "v1"
)

type ReviewInfo struct {
	Provider   string
	Number     int
	State      string
	HeadBranch string
	URL        string
}

type ReviewLookup struct {
	Reviews map[string]ReviewInfo
	Warning string
}

type reviewRemote struct {
	name     string
	url      string
	provider string
}

type reviewTarget struct {
	Branch string
	Commit string
}

type reviewCandidate struct {
	Provider   string
	Number     int
	State      string
	HeadBranch string
	HeadCommit string
	URL        string
}

type missingReviewToolError struct {
	tool string
}

func (e missingReviewToolError) Error() string {
	return e.tool + " CLI not found"
}

var reviewRunFunc = reviewRunDefault
var reviewCacheDirFunc = os.UserCacheDir

func reviewRunDefault(repoDir string, tool string, args ...string) ([]byte, error) {
	if _, err := exec.LookPath(tool); err != nil {
		return nil, missingReviewToolError{tool: tool}
	}
	cmd := exec.Command(tool, args...)
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return nil, fmt.Errorf("%s failed: %s", tool, msg)
		}
		return nil, fmt.Errorf("%s failed: %w", tool, err)
	}
	return out, nil
}

func (c *Client) ReviewStates(worktrees []Info) ReviewLookup {
	result := ReviewLookup{Reviews: map[string]ReviewInfo{}}
	if err := c.ensureInit(); err != nil {
		result.Warning = "review lookup skipped: failed to initialize repository: " + err.Error()
		return result
	}

	targets := c.reviewTargets(worktrees)
	if len(targets) == 0 {
		return result
	}

	remote, err := c.detectReviewRemote()
	if err != nil {
		result.Warning = "review lookup skipped: " + err.Error()
		return result
	}

	var reviews map[string]ReviewInfo
	switch remote.provider {
	case reviewProviderGitHub:
		reviews, err = githubReviewStates(c.repoDir, remote.url, targets)
	case reviewProviderGitLab:
		reviews, err = gitlabReviewStates(c.repoDir, remote.url, targets)
	default:
		err = fmt.Errorf("unsupported review provider for remote %q", remote.name)
	}
	if err != nil {
		result.Warning = reviewLookupWarning(remote.provider, err)
		return result
	}
	result.Reviews = reviews
	return result
}

func (c *Client) detectReviewRemote() (reviewRemote, error) {
	remotes, err := c.ListRemotes()
	if err != nil {
		return reviewRemote{}, err
	}
	if len(remotes) == 0 {
		return reviewRemote{}, errors.New("no git remotes found")
	}

	candidates := reviewRemoteCandidates(remotes)
	var unsupported []string
	for _, remote := range candidates {
		remoteURL, err := c.remoteURL(remote)
		if err != nil {
			return reviewRemote{}, err
		}
		provider := reviewProviderFromRemoteURL(remoteURL)
		if provider == "" {
			unsupported = append(unsupported, remote)
			continue
		}
		return reviewRemote{name: remote, url: remoteURL, provider: provider}, nil
	}
	if len(unsupported) > 0 {
		return reviewRemote{}, fmt.Errorf("unsupported remote host for %s", strings.Join(unsupported, ", "))
	}
	return reviewRemote{}, errors.New("no usable git remote found")
}

func (c *Client) reviewTargets(worktrees []Info) map[string]reviewTarget {
	targets := make(map[string]reviewTarget)
	mainBranch := ""
	if branch, err := c.resolvedMainBranch(); err == nil {
		mainBranch = branch
	}
	for _, wt := range worktrees {
		branch := strings.TrimSpace(wt.Branch)
		if branch == "" || branch == "(detached)" || branch == mainBranch {
			continue
		}
		if !c.branchPushedToOrigin(branch) {
			continue
		}
		if _, exists := targets[branch]; !exists {
			targets[branch] = reviewTarget{Branch: branch, Commit: wt.Commit}
		}
	}
	return targets
}

func (c *Client) branchPushedToOrigin(branch string) bool {
	if branch == "" {
		return false
	}
	_, err := c.runner.Run("-C", c.repoDir, "show-ref", "--verify", "--quiet", "refs/remotes/origin/"+branch)
	return err == nil
}

func (c *Client) remoteURL(remote string) (string, error) {
	result, err := c.runner.Run("-C", c.repoDir, "remote", "get-url", remote)
	if err != nil {
		return "", fmt.Errorf("failed to get remote URL for %s: %w", remote, err)
	}
	return result.StdoutString(true), nil
}

func reviewRemoteCandidates(remotes []string) []string {
	var candidates []string
	add := func(name string) {
		for _, remote := range remotes {
			if remote == name {
				candidates = append(candidates, remote)
				return
			}
		}
	}
	add("upstream")
	add("origin")
	if len(candidates) == 0 && len(remotes) == 1 {
		candidates = append(candidates, remotes[0])
	}
	return candidates
}

func reviewProviderFromRemoteURL(remoteURL string) string {
	host := strings.ToLower(remoteHost(remoteURL))
	switch {
	case host == "github.com" || strings.HasSuffix(host, ".github.com"):
		return reviewProviderGitHub
	case strings.Contains(host, "gitlab"):
		return reviewProviderGitLab
	case os.Getenv("GITLAB_HOST") != "" && host == strings.ToLower(os.Getenv("GITLAB_HOST")):
		return reviewProviderGitLab
	default:
		return ""
	}
}

func remoteHost(remoteURL string) string {
	trimmed := strings.TrimSpace(remoteURL)
	if parsed, err := url.Parse(trimmed); err == nil && parsed.Host != "" {
		return stripPort(parsed.Host)
	}
	if at := strings.Index(trimmed, "@"); at >= 0 {
		rest := trimmed[at+1:]
		if colon := strings.Index(rest, ":"); colon >= 0 {
			return stripPort(rest[:colon])
		}
		if slash := strings.Index(rest, "/"); slash >= 0 {
			return stripPort(rest[:slash])
		}
	}
	return ""
}

func stripPort(host string) string {
	if colon := strings.LastIndex(host, ":"); colon >= 0 {
		return host[:colon]
	}
	return host
}

type githubReviewInfo struct {
	Number      int    `json:"number"`
	State       string `json:"state"`
	HeadRefName string `json:"headRefName"`
	HeadRefOid  string `json:"headRefOid"`
	URL         string `json:"url"`
}

func githubReviewStates(
	repoDir string,
	repoURL string,
	targets map[string]reviewTarget,
) (map[string]ReviewInfo, error) {
	out, err := cachedReviewOutput(
		reviewProviderGitHub,
		repoURL,
		"me",
		func() ([]byte, error) {
			return reviewRunFunc(repoDir,
				"gh",
				"pr", "list",
				"-R", repoURL,
				"--author", "@me",
				"--state", "all",
				"--json", "number,state,headRefName,headRefOid,url",
				"--limit", "1000",
			)
		},
	)
	if err != nil {
		return nil, err
	}

	var prs []githubReviewInfo
	if err := decodeReviewJSON(out, &prs); err != nil {
		return nil, err
	}

	candidates := make([]reviewCandidate, 0, len(prs))
	for _, pr := range prs {
		candidates = append(candidates, reviewCandidate{
			Provider:   reviewProviderGitHub,
			Number:     pr.Number,
			State:      normalizeReviewState(pr.State),
			HeadBranch: pr.HeadRefName,
			HeadCommit: pr.HeadRefOid,
			URL:        pr.URL,
		})
	}
	return selectReviewCandidates(candidates, targets), nil
}

type gitlabReviewInfo struct {
	IID          int    `json:"iid"`
	State        string `json:"state"`
	SourceBranch string `json:"source_branch"`
	SHA          string `json:"sha"`
	WebURL       string `json:"web_url"`
}

type gitlabUserInfo struct {
	Username string `json:"username"`
}

func gitlabReviewStates(
	repoDir string,
	repoURL string,
	targets map[string]reviewTarget,
) (map[string]ReviewInfo, error) {
	out, err := cachedReviewOutput(
		reviewProviderGitLab,
		repoURL,
		"me",
		func() ([]byte, error) {
			username, err := gitlabCurrentUsername(repoDir)
			if err != nil {
				return nil, err
			}
			return reviewRunFunc(repoDir,
				"glab",
				"mr", "list",
				"-R", repoURL,
				"--all",
				"--author", username,
				"--output", "json",
				"--per-page", "100",
			)
		},
	)
	if err != nil {
		return nil, err
	}

	var mrs []gitlabReviewInfo
	if err := decodeReviewJSON(out, &mrs); err != nil {
		return nil, err
	}

	candidates := make([]reviewCandidate, 0, len(mrs))
	for _, mr := range mrs {
		candidates = append(candidates, reviewCandidate{
			Provider:   reviewProviderGitLab,
			Number:     mr.IID,
			State:      normalizeReviewState(mr.State),
			HeadBranch: mr.SourceBranch,
			HeadCommit: mr.SHA,
			URL:        mr.WebURL,
		})
	}
	return selectReviewCandidates(candidates, targets), nil
}

func gitlabCurrentUsername(repoDir string) (string, error) {
	out, err := reviewRunFunc(repoDir, "glab", "api", "user")
	if err != nil {
		return "", err
	}
	var user gitlabUserInfo
	if err := decodeReviewJSON(out, &user); err != nil {
		return "", err
	}
	if user.Username == "" {
		return "", errors.New("failed to determine GitLab username")
	}
	return user.Username, nil
}

func selectReviewCandidates(
	candidates []reviewCandidate,
	targets map[string]reviewTarget,
) map[string]ReviewInfo {
	reviews := make(map[string]ReviewInfo, len(targets))
	exact := make(map[string]bool, len(targets))
	for _, candidate := range candidates {
		target, ok := targets[candidate.HeadBranch]
		if !ok {
			continue
		}
		if exact[candidate.HeadBranch] {
			continue
		}
		info := ReviewInfo{
			Provider:   candidate.Provider,
			Number:     candidate.Number,
			State:      candidate.State,
			HeadBranch: candidate.HeadBranch,
			URL:        candidate.URL,
		}
		if target.Commit != "" && candidate.HeadCommit != "" && target.Commit == candidate.HeadCommit {
			reviews[candidate.HeadBranch] = info
			exact[candidate.HeadBranch] = true
			continue
		}
		if _, exists := reviews[candidate.HeadBranch]; !exists {
			reviews[candidate.HeadBranch] = info
		}
	}
	return reviews
}

func cachedReviewOutput(
	provider string,
	repoURL string,
	author string,
	load func() ([]byte, error),
) ([]byte, error) {
	if out, ok := readReviewCache(provider, repoURL, author); ok {
		return out, nil
	}
	out, err := load()
	if err != nil {
		return nil, err
	}
	writeReviewCache(provider, repoURL, author, out)
	return out, nil
}

func readReviewCache(provider string, repoURL string, author string) ([]byte, bool) {
	path, ok := reviewCachePath(provider, repoURL, author)
	if !ok {
		return nil, false
	}
	info, err := os.Stat(path)
	if err != nil || time.Since(info.ModTime()) > reviewCacheTTL {
		return nil, false
	}
	out, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return out, true
}

func writeReviewCache(provider string, repoURL string, author string, out []byte) {
	path, ok := reviewCachePath(provider, repoURL, author)
	if !ok {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}

func reviewCachePath(provider string, repoURL string, author string) (string, bool) {
	dir, err := reviewCacheDirFunc()
	if err != nil || dir == "" {
		return "", false
	}
	key := strings.Join([]string{reviewCacheVersion, provider, repoURL, author}, "\x00")
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(dir, "gmc", "reviews", fmt.Sprintf("%x.json", sum)), true
}

func decodeReviewJSON(out []byte, target any) error {
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" || trimmed == "[]" {
		return nil
	}
	if err := json.Unmarshal([]byte(trimmed), target); err != nil {
		return fmt.Errorf("failed to parse review lookup output: %w", err)
	}
	return nil
}

func normalizeReviewState(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "open", "opened":
		return "OPEN"
	case "merged":
		return "MERGED"
	case "closed", "closed_unmerged":
		return "CLOSED"
	default:
		return strings.ToUpper(strings.TrimSpace(state))
	}
}

func reviewLookupWarning(provider string, err error) string {
	tool := "review"
	name := "review"
	switch provider {
	case reviewProviderGitHub:
		tool = "gh"
		name = "GitHub PR"
	case reviewProviderGitLab:
		tool = "glab"
		name = "GitLab MR"
	}

	var missing missingReviewToolError
	if errors.As(err, &missing) {
		return fmt.Sprintf("%s lookup skipped: %s CLI not found", name, tool)
	}
	return fmt.Sprintf("%s lookup skipped: %s failed; check authentication", name, tool)
}
