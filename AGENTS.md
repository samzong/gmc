# Repository Guidelines

## Project Structure & Module Organization
Keep the CLI entry wired through `main.go`; shared Cobra commands belong in `cmd/`. Encapsulate domain logic under `internal/` (e.g., `internal/analyzer`, `internal/formatter`, `internal/git`), keeping tests beside their sources as `_test.go`. Generated binaries and coverage artifacts live in `build/`—never commit its contents. Release automation and linting configs stay in `action.yml`, `.golangci.yml`, `.goreleaser.yaml`, and `.github/`, while marketing docs live under `website/`.

## Build, Test, and Development Commands
Run `make build` to compile a static `build/gmc` binary with version metadata. Use `make fmt` to apply `go fmt ./...` and `go mod tidy`, then `make lint` (or `make lint-fix`) for the curated `golangci-lint` suite. Execute `make test` or `go test ./...` for fast verification, and `make test-coverage` when you need `build/coverage.html`. Before every push, run `make check` to chain formatting, linting, and tests.

## Coding Style & Naming Conventions
Target Go 1.24 and rely on gofmt defaults (tabs, short lines). Group imports with GCI rules enforced by `golangci-lint`. Keep package names lowercase and descriptive, scope exports conservatively, and choose explicit flag names for Cobra commands (e.g., `--config-path`, not `--cfg`).

## Testing Guidelines
Use standard `testing` with `testify` helpers for assertions. Favour table-driven cases for CLI scenarios and stub external LLMs through interfaces in `internal/llm`. Maintain existing coverage by inspecting `build/coverage.html` after substantive changes and keep regression tests near the code they exercise.

## Commit & Pull Request Guidelines
Write Conventional Commits such as `feat(cmd): add summarize command` or `fix(git): normalize repo path`. Squash noisy iterations locally. Every PR should link related issues, document configuration changes, and attach CLI output or screenshots when user-visible behaviour shifts. Always run `make check` before requesting review.

## Security & Configuration Tips
Record new model or API requirements via `gmc config set ...` examples in docs or PRs so teammates can reproduce setups. Never commit credentials; prefer environment variables sourced through Viper and local `.env` files ignored by `.gitignore`.
