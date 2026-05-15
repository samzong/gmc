# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

gmc is a Go CLI for the AI coding era: **parallel worktrees for parallel AI agents — plus AI-generated commits**. The primary surface is worktree management built on a `.bare` clone (`gmc wt add/dup/share/...`), letting you run multiple Claude Code / Codex / Copilot agents side by side with shared `.env` and `node_modules`. Secondary, it uses an LLM to generate Conventional Commits messages from staged diffs.

## Key Components

### Architecture

**三层模型**（详见 [`docs/COBRA_GUIDE.md`](docs/COBRA_GUIDE.md)）：

```
Layer 1: Command Definition (cmd/*.go)
    │   cobra.Command + Flag 绑定 + Args 验证
    ▼
Layer 2: Runner Function
    │   构建 Options → 调用 Layer 3 → 格式化输出
    ▼
Layer 3: Business Logic (internal/*)
        无 CLI 依赖，可独立测试
```

- **Entry Point**: `main.go` - 调用 `cmd.Execute()`，统一处理错误输出
- **CLI Framework**: Cobra (`cmd/` package)
  - `cmd/root.go` - 根命令 + commit 工作流
  - `cmd/config.go` - 配置管理命令组
  - `cmd/worktree.go` - worktree 命令组
  - `cmd/output.go` - 输出流抽象
- **Core Modules** (`internal/`):
  - `config/` - 配置管理 (Viper)
  - `git/` - Git 操作封装
  - `gitcmd/` - Git 命令执行抽象
  - `gitutil/` - Git 工具函数
  - `llm/` - LLM API 集成
  - `formatter/` - Prompt 构建 + 消息格式化
  - `workflow/` - 提交工作流编排
  - `worktree/` - Worktree 操作
  - `branch/` - 分支命名生成
  - `emoji/` - Gitmoji 支持
  - `exitcode/` - 结构化退出码
  - `shell/` - Shell 集成
  - `ui/` - 终端交互
  - `stringsutil/` - 字符串工具
  - `version/` - 版本信息

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
   - `e` - Edit in external editor ($EDITOR or $VISUAL, defaults to vi)
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
- **Editor Integration**: Creates temp file, opens with system editor, reads edited content
- **Error Handling**: 使用 `userFacingError` 包装用户可见错误，保留原始错误链供 `errors.Is()` 检查

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

核心规范摘要：

| 规范 | 要求 |
|------|------|
| **命令定义** | 使用 `RunE` 而非 `Run`；必须设置 `Args` 验证器 |
| **Flag 命名** | 长名用连字符 `--dry-run`，禁止驼峰或下划线 |
| **输出流** | stdout = 数据，stderr = 进度/错误；使用 `cmd.OutOrStdout()` |
| **错误处理** | 返回 `error`，由 `main.go` 统一输出 |
| **测试** | 避免全局状态；使用工厂函数创建隔离命令实例 |
| **补全** | 新命令必须配置 `ValidArgsFunction` 或 `ValidArgs` |

**新命令 PR 检查项**：
- [ ] `Use` 字段遵循 POSIX 语法：`command <required> [optional]`
- [ ] `Short` < 50 字符，有 `Example` 字段
- [ ] Flag 关系已声明（互斥/共存/必选其一）
- [ ] 有对应测试用例
- [ ] Shell 补全已配置

### Branch Creation Feature
- New `--branch` flag creates feature branches with generated names based on description
- Uses `internal/branch/` package for branch name generation and Git operations
- Integrates with existing commit workflow for seamless development experience

@AGENTS.md
