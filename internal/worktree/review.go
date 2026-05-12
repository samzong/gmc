package worktree

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
)

const (
	reviewProviderGitHub = "github"
	reviewProviderGitLab = "gitlab"
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

type missingReviewToolError struct {
	tool string
}

func (e missingReviewToolError) Error() string {
	return e.tool + " CLI not found"
}

var reviewRunFunc = reviewRunDefault

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

func (c *Client) ReviewStates() ReviewLookup {
	result := ReviewLookup{Reviews: map[string]ReviewInfo{}}
	if err := c.ensureInit(); err != nil {
		result.Warning = "review lookup skipped: failed to initialize repository: " + err.Error()
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
		reviews, err = githubReviewStates(c.repoDir, remote.url)
	case reviewProviderGitLab:
		reviews, err = gitlabReviewStates(c.repoDir, remote.url)
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
	URL         string `json:"url"`
}

func githubReviewStates(repoDir string, repoURL string) (map[string]ReviewInfo, error) {
	out, err := reviewRunFunc(repoDir,
		"gh",
		"pr", "list",
		"-R", repoURL,
		"--state", "all",
		"--json", "number,state,headRefName,url",
		"--limit", "300",
	)
	if err != nil {
		return nil, err
	}

	var prs []githubReviewInfo
	if err := decodeReviewJSON(out, &prs); err != nil {
		return nil, err
	}

	reviews := make(map[string]ReviewInfo, len(prs))
	for _, pr := range prs {
		if _, exists := reviews[pr.HeadRefName]; exists {
			continue
		}
		reviews[pr.HeadRefName] = ReviewInfo{
			Provider:   reviewProviderGitHub,
			Number:     pr.Number,
			State:      normalizeReviewState(pr.State),
			HeadBranch: pr.HeadRefName,
			URL:        pr.URL,
		}
	}
	return reviews, nil
}

type gitlabReviewInfo struct {
	IID          int    `json:"iid"`
	State        string `json:"state"`
	SourceBranch string `json:"source_branch"`
	WebURL       string `json:"web_url"`
}

func gitlabReviewStates(repoDir string, repoURL string) (map[string]ReviewInfo, error) {
	out, err := reviewRunFunc(repoDir,
		"glab",
		"mr", "list",
		"-R", repoURL,
		"--all",
		"--output", "json",
		"--per-page", "100",
	)
	if err != nil {
		return nil, err
	}

	var mrs []gitlabReviewInfo
	if err := decodeReviewJSON(out, &mrs); err != nil {
		return nil, err
	}

	reviews := make(map[string]ReviewInfo, len(mrs))
	for _, mr := range mrs {
		if _, exists := reviews[mr.SourceBranch]; exists {
			continue
		}
		reviews[mr.SourceBranch] = ReviewInfo{
			Provider:   reviewProviderGitLab,
			Number:     mr.IID,
			State:      normalizeReviewState(mr.State),
			HeadBranch: mr.SourceBranch,
			URL:        mr.WebURL,
		}
	}
	return reviews, nil
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
