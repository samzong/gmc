# Repository Guidelines

## Project Structure & Module Organization
Keep the CLI entry wired through `main.go`; shared Cobra commands belong in `cmd/`. Encapsulate domain logic under `internal/` with the following modules:
- `internal/analyzer`: Commit history analysis and quality assessment
- `internal/branch`: Branch name generation and management
- `internal/config`: Configuration management using Viper
- `internal/formatter`: Commit message formatting and prompt template handling
- `internal/git`: Git operations abstraction and safety checks
- `internal/llm`: LLM API integration and message generation

Keep tests beside their sources as `_test.go`. Generated binaries and coverage artifacts live in `build/`—never commit its contents. Release automation and linting configs stay in `action.yml`, `.golangci.yml`, `.goreleaser.yaml`, and `.github/`, while marketing docs live under `website/`.

### CLI Commands Structure
- `cmd/root.go`: Main command entry with flags (`--all`, `--issue`, `--branch`, `--dry-run`, `--no-verify`, `--no-signoff`, `--yes`, `--verbose`)
- `cmd/analyze.go`: Commit history analysis command (`gmc analyze`, `gmc analyze --team`)
- `cmd/config.go`: Configuration management (`gmc config set/get/list_templates`)
- `cmd/version.go`: Version information command (`gmc version`)

## Build, Test, and Development Commands
Run `make build` to compile a static `build/gmc` binary with version metadata. Use `make fmt` to apply `go fmt ./...` and `go mod tidy`, then `make lint` (or `make lint-fix`) for the curated `golangci-lint` suite. Execute `make test` or `go test ./...` for fast verification, and `make test-coverage` when you need `build/coverage.html`. Before every push, run `make check` to chain formatting, linting, and tests.

### Available Make Targets
- `make build`: Build binary with version metadata
- `make install`: Install to GOPATH/bin
- `make clean`: Remove build artifacts
- `make test`: Run all tests
- `make test-coverage`: Generate coverage report
- `make fmt`: Format code and tidy modules
- `make lint`: Run golangci-lint
- `make lint-fix`: Run golangci-lint with auto-fix
- `make check`: Run fmt, lint, and test in sequence

## Coding Style & Naming Conventions
Target Go 1.24 and rely on gofmt defaults (tabs, short lines). Group imports with GCI rules enforced by `golangci-lint`. Keep package names lowercase and descriptive, scope exports conservatively, and choose explicit flag names for Cobra commands (e.g., `--config-path`, not `--cfg`).

## Testing Guidelines
Use standard `testing` with `testify` helpers for assertions. Favour table-driven cases for CLI scenarios and stub external LLMs through interfaces in `internal/llm`. Maintain existing coverage by inspecting `build/coverage.html` after substantive changes and keep regression tests near the code they exercise.

### Test Organization
- Unit tests: Co-located with source files as `*_test.go`
- Test helpers: Located in `internal/git/test_helper.go` for git-related tests
- Mock interfaces: Use interfaces in `internal/llm` for LLM stubbing

## Commit & Pull Request Guidelines
Write Conventional Commits such as `feat(cmd): add summarize command` or `fix(git): normalize repo path`. Squash noisy iterations locally. Every PR should link related issues, document configuration changes, and attach CLI output or screenshots when user-visible behaviour shifts. Always run `make check` before requesting review.

## Security & Configuration Tips
Record new model or API requirements via `gmc config set ...` examples in docs or PRs so teammates can reproduce setups. Never commit credentials; prefer environment variables sourced through Viper and local `.env` files ignored by `.gitignore`.

### Configuration Commands
- `gmc config set apikey <key>`: Set OpenAI API key
- `gmc config set apibase <url>`: Set API base URL for proxy
- `gmc config set model <model>`: Set LLM model
- `gmc config set role <role>`: Set current role
- `gmc config set prompt_template <name>`: Set prompt template
- `gmc config set prompts_dir <path>`: Set prompt template directory
- `gmc config get`: Display current configuration
- `gmc config list_templates`: List available templates
