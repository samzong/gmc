# Changelog


## [v0.6.1] - 2026-01-07
### Bug Fixes
- support worktree commands in bare workspace root


## [v0.6.0] - 2026-01-07
### Features
- **worktree:** add support for shared resources across worktrees
- **worktree:** add sync command and --sync option
- **worktree:** add prune command to clean merged worktrees


## [v0.5.0] - 2025-12-27
### Code Refactoring
- **cmd:** pass git/llm clients as args, add tests & update man pages


## [v0.4.3] - 2025-12-26
### Code Refactoring
- **config:** use RunE and split command logic
- **core:** extract saveConfig and logVerboseOutput helpers


## [v0.4.2] - 2025-12-26
### Code Refactoring
- **cmd:** drop context, tidy imports & prompts
- **config:** redirect messages to stderr, secure API key entry

### Features
- **init:** add interactive init command to configure API key and LLM model
- **init:** add interactive init command for configuring API key and model


## [v0.4.1] - 2025-12-22
### Features
- **config:** merge repo-level config with higher priority


## [v0.4.0] - 2025-12-21
### Code Refactoring
- **git:** remove test environment commit safety checks and related test files
- **git:** unify git command execution via gitcmd.Runner integration

### Features
- **worktree:** add 'dup' and 'promote' commands for parallel worktrees


## [v0.3.2] - 2025-12-15

## [v0.3.1] - 2025-12-12
### Bug Fixes
- **docker:** use alpine:3.19 to avoid QEMU cross-arch trigger issues


## [v0.3.0] - 2025-12-12
### Bug Fixes
- **worktree:** refine bare worktree filtering and fetch error handling

### Code Refactoring
- **cmd:** improve error handling to propagate sentinel errors and print to stderr

### Features
- **worktree:** add new 'wt' command for managing git worktrees


## [v0.2.0] - 2025-11-06
### Features
- **tag:** add interactive semantic version tagging command with LLM integration


## [v0.1.4] - 2025-11-04
### Features
- **emoji:** commit message add emoji support


## [v0.1.3] - 2025-11-04
### Code Refactoring
- **cmd:** replace unused command arguments with underscores for clarity

### Features
- **ci:** add multi-arch Docker build and GitHub Container Registry support


## [v0.1.2] - 2025-10-31
### Features
- **core:** add user prompt support for commit message generation


## [v0.1.1] - 2025-09-17
### Code Refactoring
- **git:** replace if-else with switch in CheckFileStatus function

### Features
- add comprehensive .github/copilot-instructions.md with validated commands and scenarios


## [v0.1.0] - 2025-08-28
### Features
- **action:** add initial GitHub Action definition for GMC commit message tool


## [v0.0.8] - 2025-08-15
### Features
- **cmd:** add selective file commit support with staging and validation
- **git:** add file resolution and status checking utilities


## [v0.0.7] - 2025-08-13
### Bug Fixes
- **errors:** standardize error messages to lowercase for consistency

### Features
- **analyzer:** implement commit history analysis with AI-powered suggestions and quality metrics
- **test:** add test coverage commands and safety checks


## [v0.0.6] - 2025-08-13
### Code Refactoring
- **build:** change build directory from ./bin to ./build for better organization

### Features
- **cmd:** add branch creation option with generated name based on description


## [v0.0.5] - 2025-08-08
### Features
- **docs:** add CLAUDE.md guide and detailed feature enhancement roadmap
- **git:** add verbose flag to show detailed git command output and support verbose mode


## [v0.0.4] - 2025-05-15
### Features
- **cmd:** add option to edit commit message before confirming
- **release:** add v0.0.4 changelog and remove changelog generation hook from goreleaser config


## [v0.0.3] - 2025-05-15
### Code Refactoring
- **git:** add repository check in git command functions to ensure valid context

### Features
- **cmd:** add auto-confirm flag and interactive commit confirmation


## [v0.0.2] - 2025-05-14
### Bug Fixes
- correct template format in changelog

### Code Refactoring
- **config:** rename custom_prompts_dir to prompts_dir and update related prompt template references
- **config:** rename CustomPromptsDir to PromptsDir and update related defaults and usages

### Features
- add changelog generation support


## v0.0.1 - 2025-05-13
### Code Refactoring
- **build:** rename binary and update configs from gma to gmc across build, CI, and docs
- **config:** unify configuration set commands and improve error handling

### Features
- **build:** add GoReleaser config, enhance Makefile with build time, formatting, testing, and help targets; improve CLI config handling
- **ci:** add GitHub workflows for PR review, release, and Homebrew tap update with Makefile integration
- **gma:** add option to automatically stage all changes ([#123](https://github.com/samzong/gmc/issues/123))


[Unreleased]: https://github.com/samzong/gmc/compare/v0.6.1...HEAD
[v0.6.1]: https://github.com/samzong/gmc/compare/v0.6.0...v0.6.1
[v0.6.0]: https://github.com/samzong/gmc/compare/v0.5.0...v0.6.0
[v0.5.0]: https://github.com/samzong/gmc/compare/v0.4.3...v0.5.0
[v0.4.3]: https://github.com/samzong/gmc/compare/v0.4.2...v0.4.3
[v0.4.2]: https://github.com/samzong/gmc/compare/v0.4.1...v0.4.2
[v0.4.1]: https://github.com/samzong/gmc/compare/v0.4.0...v0.4.1
[v0.4.0]: https://github.com/samzong/gmc/compare/v0.3.2...v0.4.0
[v0.3.2]: https://github.com/samzong/gmc/compare/v0.3.1...v0.3.2
[v0.3.1]: https://github.com/samzong/gmc/compare/v0.3.0...v0.3.1
[v0.3.0]: https://github.com/samzong/gmc/compare/v0.2.0...v0.3.0
[v0.2.0]: https://github.com/samzong/gmc/compare/v0.1.4...v0.2.0
[v0.1.4]: https://github.com/samzong/gmc/compare/v0.1.3...v0.1.4
[v0.1.3]: https://github.com/samzong/gmc/compare/v0.1.2...v0.1.3
[v0.1.2]: https://github.com/samzong/gmc/compare/v0.1.1...v0.1.2
[v0.1.1]: https://github.com/samzong/gmc/compare/v0.1.0...v0.1.1
[v0.1.0]: https://github.com/samzong/gmc/compare/v0.0.8...v0.1.0
[v0.0.8]: https://github.com/samzong/gmc/compare/v0.0.7...v0.0.8
[v0.0.7]: https://github.com/samzong/gmc/compare/v0.0.6...v0.0.7
[v0.0.6]: https://github.com/samzong/gmc/compare/v0.0.5...v0.0.6
[v0.0.5]: https://github.com/samzong/gmc/compare/v0.0.4...v0.0.5
[v0.0.4]: https://github.com/samzong/gmc/compare/v0.0.3...v0.0.4
[v0.0.3]: https://github.com/samzong/gmc/compare/v0.0.2...v0.0.3
[v0.0.2]: https://github.com/samzong/gmc/compare/v0.0.1...v0.0.2
