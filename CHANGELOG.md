# Changelog

## [Unreleased]

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


[Unreleased]: https://github.com/samzong/gmc/compare/v0.0.1...HEAD
