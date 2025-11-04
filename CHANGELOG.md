# Changelog


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


[Unreleased]: https://github.com/samzong/gmc/compare/v0.1.3...HEAD
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
