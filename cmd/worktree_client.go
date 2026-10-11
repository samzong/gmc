package cmd

import (
	"github.com/samzong/gmc/internal/config"
	"github.com/samzong/gmc/internal/worktree"
)

func newWorktreeClient() *worktree.Client {
	return newWorktreeClientAt("")
}

func newWorktreeClientAt(dir string) *worktree.Client {
	opts := worktree.Options{Dir: dir, Verbose: verbose || debug, GlobalConfigPath: config.FilePath()}
	if outputFormat() == "json" {
		opts.HookOutput = errWriter()
	}
	return worktree.NewClient(opts)
}
