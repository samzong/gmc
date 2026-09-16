package worktree

import (
	"errors"
)

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
	targets map[string]string,
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
	targets map[string]string,
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
