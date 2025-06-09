# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

GMC (Git Message Commit) is a CLI tool written in Go that uses LLM to generate high-quality Git commit messages following Conventional Commits specification. The tool helps developers by automatically analyzing Git diffs and generating appropriate commit messages.

## Key Components

### Architecture
- **Entry Point**: `main.go` - Simple entry point that calls `cmd.Execute()`
- **CLI Framework**: Uses Cobra for command structure (`cmd/` package)
- **Core Modules**:
  - `internal/config/` - Configuration management using Viper
  - `internal/git/` - Git operations (diff, add, commit)
  - `internal/llm/` - OpenAI API integration for message generation
  - `internal/formatter/` - Prompt building and message formatting

### Command Structure
- Root command: `gmc` - Main commit functionality
- Subcommand: `gmc config` - Configuration management with subcommands:
  - `set role/model/apikey/apibase/prompt_template/prompts_dir`
  - `get` - Show current configuration
  - `list_templates` - List available prompt templates

## Development Commands

### Build and Development
```bash
# Build the binary
make build

# Format code and tidy modules
make fmt

# Run tests
make test

# Build and install to GOPATH
make install

# Complete build cycle (clean, format, build, test)
make all

# Clean build artifacts
make clean
```

### Configuration
The tool stores configuration in `~/.gmc.yaml` and supports:
- OpenAI API credentials (`apikey`, `apibase`)
- LLM model selection (`model`)
- Developer role for prompt context (`role`)
- Custom prompt templates (`prompt_template`, `prompts_dir`)

### Core Workflow
1. Analyzes Git staged changes using `git diff --cached`
2. Builds prompt using role, changed files, and diff content
3. Calls OpenAI API to generate commit message
4. Formats message to follow Conventional Commits specification
5. Provides interactive confirmation with options (yes/no/regenerate/edit)
6. Commits with `git commit -m`

### Important Implementation Details
- Diff content is truncated to 4000 characters to avoid token limits
- Supports custom prompt templates in YAML format
- Template variables: `{{.Role}}`, `{{.Files}}`, `{{.Diff}}`
- Built-in conventional commit type detection and formatting
- Issue number integration via `--issue` flag
- Editor integration for manual message editing (uses $EDITOR or $VISUAL)

### Dependencies
- Cobra + Viper for CLI and configuration
- `github.com/sashabaranov/go-openai` for OpenAI API
- Standard library for Git operations via `os/exec`