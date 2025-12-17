# Repository Guidelines

## Project Structure & Module Organization

This is a Go CLI (Cobra + Viper). The entrypoint is `main.go`; CLI commands live in `cmd/`, and domain logic is encapsulated under `internal/`:

- `internal/analyzer`: commit history analysis / quality assessment
- `internal/branch`: branch name generation / management
- `internal/config`: configuration via Viper
- `internal/formatter`: commit message formatting + prompt templates
- `internal/git`: git operations + safety checks
- `internal/llm`: LLM integration + message generation

Tests are colocated with sources as `*_test.go`. Generated binaries and coverage artifacts belong in `build/` (do not commit its contents). Repo automation lives in `action.yml`, `.golangci.yml`, `.goreleaser.yaml`, and `.github/`.

## Build, Test, and Development Commands

- `make build`: build static `build/gmc` with version metadata
- `make test` (or `go test ./...`): run unit tests
- `make test-coverage`: generate `build/coverage.html`
- `make fmt`: run `go fmt ./...` and `go mod tidy`
- `make lint` / `make lint-fix`: run the curated `golangci-lint` suite
- `make check`: run fmt + lint + test (run before pushing)

## Coding Style & Naming Conventions

Target Go 1.24 and rely on `gofmt` defaults (tabs, short lines). Imports are grouped per GCI rules enforced by `golangci-lint`. Keep package names lowercase and descriptive; prefer minimal exports. Cobra flags should be explicit (e.g., `--config-path`, not `--cfg`).

## Testing Guidelines

Use the standard `testing` package with `testify` for assertions. Prefer table-driven tests for CLI behavior. Stub LLM calls via interfaces in `internal/llm` (avoid network calls in tests).

## Commit & Pull Request Guidelines

Use Conventional Commits (e.g., `feat(cmd): add summarize command`, `fix(git): normalize repo path`). PRs should link issues, describe behavior/config changes, and include CLI output (or screenshots) for user-visible changes. Run `make check` before requesting review.

## Security & Configuration Tips

Never commit credentials. Configuration is managed via Viper; set local values with:

- `gmc config set apikey <key>`
- `gmc config set apibase <url>`
- `gmc config set model <model>`

