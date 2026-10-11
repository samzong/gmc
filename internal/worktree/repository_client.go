package worktree

import (
	"fmt"
	"os"
	"slices"
	"strings"
)

var repositoryEnvVars = []string{
	"GIT_ALTERNATE_OBJECT_DIRECTORIES",
	"GIT_CONFIG",
	"GIT_CONFIG_PARAMETERS",
	"GIT_CONFIG_COUNT",
	"GIT_OBJECT_DIRECTORY",
	"GIT_DIR",
	"GIT_WORK_TREE",
	"GIT_IMPLICIT_WORK_TREE",
	"GIT_GRAFT_FILE",
	"GIT_INDEX_FILE",
	"GIT_NO_REPLACE_OBJECTS",
	"GIT_REPLACE_REF_BASE",
	"GIT_PREFIX",
	"GIT_SHALLOW_FILE",
	"GIT_COMMON_DIR",
}

func NewRepositoryClient(repository string, opts Options) (*Client, error) {
	opts.Dir = repository
	client := NewClient(opts)
	client.runner.Env = repositoryEnvironment()
	root, err := client.GetWorktreeRoot()
	if err != nil {
		return nil, err
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	repositoryInfo, err := os.Stat(repository)
	if err != nil {
		return nil, err
	}
	if !os.SameFile(rootInfo, repositoryInfo) {
		return nil, fmt.Errorf("resolved worktree root %s does not match scanned repository", root)
	}
	return client, nil
}

func repositoryEnvironment() []string {
	env := []string{"GIT_OPTIONAL_LOCKS=0"}
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		if name != "GIT_OPTIONAL_LOCKS" && name != "GH_REPO" && !slices.Contains(repositoryEnvVars, name) {
			env = append(env, value)
		}
	}
	return env
}
