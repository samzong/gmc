package cmd

import "github.com/samzong/gmc/internal/worktree"

func newWorktreeClient() *worktree.Client {
	return worktree.NewClient(worktree.Options{Verbose: verbose})
}
