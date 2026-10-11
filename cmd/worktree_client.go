package cmd

import (
	"github.com/samzong/gmc/internal/config"
	"github.com/samzong/gmc/internal/worktree"
)

func newWorktreeClient() *worktree.Client {
	return worktree.NewClient(worktreeClientOptions())
}

func worktreeClientOptions() worktree.Options {
	opts := worktree.Options{Verbose: verbose || debug, GlobalConfigPath: config.FilePath()}
	if outputFormat() == "json" {
		opts.HookOutput = errWriter()
	}
	return opts
}
