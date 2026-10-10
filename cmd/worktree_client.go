package cmd

import (
	"github.com/samzong/gmc/internal/config"
	"github.com/samzong/gmc/internal/worktree"
)

func newWorktreeClient() *worktree.Client {
	opts := worktree.Options{Verbose: verbose || debug, GlobalConfigPath: config.FilePath()}
	if outputFormat() == "json" {
		opts.HookOutput = errWriter()
	}
	return worktree.NewClient(opts)
}
