# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

gmc is a Go CLI for the AI coding era: **parallel worktrees for parallel AI agents — plus AI-generated commits**. The primary surface is worktree management built on a `.bare` clone (`gmc wt add/dup/share/...`), letting you run multiple Claude Code / Codex / Copilot agents side by side with shared `.env` and `node_modules`. Secondarily, it uses an LLM to generate Conventional Commits messages from staged diffs.

## Key Components

### Architecture

**Three-layer model** (see [`docs/COBRA_GUIDE.md`](docs/COBRA_GUIDE.md) for details):

```
Layer 1: Command Definition (cmd/*.go)
    │   cobra.Command + flag binding + args validation
    ▼
Layer 2: Runner Function
    │   builds Options → calls Layer 3 → formats output
    ▼
Layer 3: Business Logic (internal/*)
        no CLI dependency; independently testable
```

- **Entry Point**: `main.go` - calls `cmd.Execute()` and centralizes error output
- **CLI Framework**: Cobra (`cmd/` package)
  - `cmd/root.go` - root command + commit workflow
  - `cmd/config.go` - configuration command group
  - `cmd/worktree.go` - worktree command group
  - `cmd/output.go` - output stream abstraction
- **Core Modules** (`internal/`):
  - `config/` - configuration management (Viper)
  - `git/` - Git operation wrappers
  - `gitcmd/` - Git command execution abstraction
  - `gitutil/` - Git utility functions
  - `llm/` - LLM API integration
  - `formatter/` - prompt construction + message formatting
  - `workflow/` - commit workflow orchestration
  - `worktree/` - worktree operations
  - `branch/` - branch name generation
  - `emoji/` - Gitmoji support
  - `exitcode/` - structured exit codes
  - `shell/` - shell integration
  - `ui/` - terminal interactions
  - `stringsutil/` - string utilities
  - `version/` - version information

### Command Structure
- Root command: `gmc` - Main commit functionality
  - Flags: `-a/--all`, `--issue`, `-y/--yes`, `--no-verify`, `--no-signoff`, `--dry-run`, `--branch`, `--prompt`, `--verbose`, `--output`, `--config`
- Subcommand: `gmc init` - Interactive setup wizard
- Subcommand: `gmc config` - Configuration management with subcommands:
  - `set role/model/apikey/apibase/prompt_template/enable_emoji`
  - `get` - Show current configuration
- Subcommand: `gmc tag` - Semantic version tag suggestion
- Subcommand: `gmc wt` - Worktree management (add, remove, list, clone, dup, promote, prune, sync, share, pr-review, switch)
- Subcommand: `gmc version` - Display version and build information

## Development Commands

### Build and Development
```bash
# Build the binary
make build

# Format code and tidy modules
make fmt

# Run linter
make lint

# Run tests
make test

# Run all quality checks (fmt, lint, test)
make check

# Build and install to GOPATH/bin
make install

# Clean build artifacts
make clean

# Generate man pages
make man

# Update Homebrew formula (requires GH_PAT env var)
make update-homebrew
```

### Testing a Single Component
```bash
# Run tests for a specific package
go test ./internal/formatter -v

# Run tests with coverage
go test -cover ./...

# Run tests for branch functionality specifically
go test ./internal/branch -v

# Test with race detection
go test -race ./...
```

### Configuration
The tool stores configuration in `~/.gmc.yaml` and supports:
- OpenAI API credentials (`apikey`, `apibase`)
- LLM model selection (`model`)
- Developer role for prompt context (`role`)
- Custom prompt templates (`prompt_template`, `prompts_dir`)

### Core Workflow
1. Optionally adds all changes with `git add --all` (when `-a` flag is used)
2. Retrieves staged diff using `git diff --cached`
3. Parses staged files list
4. Builds prompt using role, changed files, and diff content (max 4000 chars)
5. Calls OpenAI API to generate commit message
6. Formats message to follow Conventional Commits specification
7. Provides interactive confirmation with options:
   - `y` - Confirm and commit
   - `n` - Cancel
   - `r` - Regenerate message
   - `e` - Edit in external editor (`$EDITOR` or `$VISUAL`, defaults to `vi`)
8. Commits with `git commit -m` (adds `--no-verify` if flag is set)

### Important Implementation Details
- **Diff Truncation**: Diff content is truncated to 4000 characters in `formatter.BuildPrompt()` to avoid token limits
- **Template System**:
  - Templates stored in `~/.gmc/prompts/` directory
  - YAML format with `name`, `description`, and `template` fields
  - Template variables: `{{.Role}}`, `{{.Files}}`, `{{.Diff}}`
  - Fallback to built-in "default" template if custom template fails
- **Message Formatting**:
  - Automatic removal of issue numbers from LLM output (handled separately)
  - Type detection based on keywords if not in conventional format
  - Issue number appended as ` (#123)` when `--issue` flag is used
- **Editor Integration**: Creates temp file, opens it with the system editor, then reads the edited content
- **Error Handling**: Wrap user-visible errors with `userFacingError` while preserving the original error chain for `errors.Is()` checks

### Dependencies
- Cobra v1.10.2 + Viper v1.21.0 for CLI and configuration
- `github.com/sashabaranov/go-openai` v1.41.2 for OpenAI API
- Standard library for Git operations via `os/exec`
- Go 1.24+ required

### Critical Development Notes

**Production Impact**: GMC is a widely-used Git commit tool with 1M+ daily users. Any modifications must be thoroughly tested and backward-compatible.

### Release Process
- Releases are managed via GoReleaser with automated GitHub releases
- Homebrew formula updates via `make update-homebrew` (requires `GH_PAT` env var)
- Version info is injected at build time using Git tags and build timestamp
- Supports multiple architectures: Darwin (x86_64, arm64), Linux (x86_64, arm64)

### Code Quality Standards
- All changes must pass: `make check` before submission
- Follow Go conventions and existing code patterns
- Maintain compatibility with Go 1.24+
- Test with real Git repositories to ensure proper diff handling

### Cobra CLI Development Standards

**MANDATORY**: All CLI code changes MUST follow [`docs/COBRA_GUIDE.md`](docs/COBRA_GUIDE.md).

Core standards summary:

| Standard | Requirement |
|------|------|
| **Command definition** | Use `RunE` instead of `Run`; every command must set an `Args` validator |
| **Flag naming** | Use kebab-case long names such as `--dry-run`; camelCase and snake_case are forbidden |
| **Output streams** | stdout = data, stderr = progress/errors; use `cmd.OutOrStdout()` |
| **Error handling** | Return `error`; let `main.go` centralize output |
| **Testing** | Avoid global state; use factory functions to create isolated command instances |
| **Completion** | New commands must configure `ValidArgsFunction` or `ValidArgs` |

**New command PR checklist**:
- [ ] `Use` field follows POSIX syntax: `command <required> [optional]`
- [ ] `Short` is under 50 characters and the command has an `Example` field
- [ ] Flag relationships are declared (mutually exclusive, allowed together, or at least one required)
- [ ] Corresponding tests are included
- [ ] Shell completion is configured

### Branch Creation Feature
- New `--branch` flag creates feature branches with generated names based on description
- Uses `internal/branch/` package for branch name generation and Git operations
- Integrates with existing commit workflow for seamless development experience

@AGENTS.md
