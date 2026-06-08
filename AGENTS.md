# AGENTS.md

Instructions for AI coding agents working in this repository.

## What this repo is

gmc is a Go CLI for parallel AI-agent development: **worktree management is the primary surface**, AI-generated Conventional Commits are secondary.

- **Primary**: `.bare` clone + sibling worktrees (`gmc wt add/dup/share/sync/promote/...`)
- **Secondary**: LLM commit messages from staged diffs (root `gmc` command)

User-facing command reference: [README.md](README.md). Runtime usage patterns: [skills/gmc/SKILL.md](skills/gmc/SKILL.md).

## Architecture invariants

Three layers — do not blur boundaries:

```
Layer 1: Command Definition (cmd/*.go)
    cobra.Command + flag binding + Args validation
         │
Layer 2: Runner Function
    build Options → call Layer 3 → format output
         │
Layer 3: Business Logic (internal/*)
    no CLI dependencies; testable in isolation
```

**Rules agents must follow:**

| Rule | Detail |
|------|--------|
| **Dependency direction** | `cmd/` imports `internal/`; `internal/` must not import `cmd/` or Cobra |
| **Command handlers** | Use `RunE`, not `Run`; always set an `Args` validator |
| **Output streams** | stdout = data; stderr = progress/errors; use `cmd.OutOrStdout()` / `errWriter()` |
| **Errors** | Return `error`; `main.go` prints it. Use `exitcode` for structured exit codes; `userFacingError` in `cmd/root.go` for generic wrapping |
| **Worktree CLI** | All worktree operations go through `gmc wt <subcommand>`. Never invent top-level `gmc add`, `gmc clone`, etc. |
| **Generated docs** | Never hand-edit `docs/man/*.1`. Man pages are generated from Cobra definitions in `cmd/*.go` via `make man` (`cmd/gendoc/main.go`). |

Entry point: `main.go` → `cmd.Execute()`.

## Where to change what

| Area | Location | Notes |
|------|----------|-------|
| Root commit workflow | `cmd/root.go`, `internal/workflow/` | Staging, prompt, interactive confirm, commit |
| Worktree commands | `cmd/worktree*.go`, `internal/worktree/` | Split across `worktree.go`, `worktree_share.go`, `worktree_hook.go`, `worktree_sync.go`, `worktree_init.go`, `worktree_prune.go` |
| Worktree client wiring | `cmd/worktree_client.go` | Thin factory over `worktree.NewClient` |
| Config | `cmd/config.go`, `internal/config/` | Viper-based; XDG paths |
| LLM integration | `internal/llm/` | OpenAI-compatible client |
| Prompt / formatting | `internal/formatter/` | Templates, diff truncation (`diff_truncator.go`) |
| Git operations | `internal/git/`, `internal/gitcmd/`, `internal/gitutil/` | |
| Branch naming | `internal/branch/` | `--branch` flag on root command |
| Shell integration | `internal/shell/`, `cmd/worktree_init.go` | `gmc wt init bash\|zsh\|fish` |
| Tests for CLI | `cmd/*_test.go` | Use isolated command instances; swap `outWriterFunc` / `errWriterFunc` |
| Man pages (generated) | `docs/man/*.1` | **Do not edit.** Source of truth is `Use`/`Short`/`Long`/`Example` on commands in `cmd/*.go`. Regenerate: `make man` |

When adding a worktree feature: implement logic in `internal/worktree/`, wire it in the matching `cmd/worktree_*.go` file, add tests in both packages.

## CLI conventions

Mandatory for any new or changed command:

- **Flag naming**: long flags use hyphens (`--dry-run`), not camelCase or underscores
- **`Use` syntax**: POSIX style — `command <required> [optional]`
- **`Short`**: under 50 characters; include an `Example` field for non-trivial commands
- **Flag relationships**: declare mutual exclusion / required-together with Cobra helpers
- **Completion**: configure `ValidArgsFunction` or `ValidArgs` for new commands
- **Tests**: add `cmd/*_test.go` coverage; avoid relying on global `rootCmd` state when possible — use `RootCmd()` or local command trees

PR checklist for new commands:

- [ ] `RunE` + `Args` validator
- [ ] Flag naming and relationships declared
- [ ] Shell completion configured
- [ ] Tests added
- [ ] CLI help text updated in `cmd/*.go`, then `make man` run (never patch `docs/man/` by hand)

## Generated files — do not edit

`docs/man/*.1` are **build artifacts**, not source. They are produced by `cobra/doc` from the live command tree:

```
cmd/*.go  (Cobra Use / Short / Long / Example / flags)
    → make man  (runs cmd/gendoc/main.go)
    → docs/man/*.1
```

When CLI help or flags change:

1. Edit the Cobra command definition in `cmd/*.go`
2. Run `make man`
3. Commit both the `cmd/` changes and the regenerated `docs/man/` output

Hand-editing man pages will be overwritten on the next `make man` and creates drift from the actual CLI.

## Config and runtime facts

Do not assume legacy paths or removed config keys.

**Config file resolution** (first match wins):

1. `--config` flag
2. `GMC_CONFIG` env var
3. `$XDG_CONFIG_HOME/gmc/config.yaml` (default: `~/.config/gmc/config.yaml`)
4. `~/.gmc.yaml` (legacy fallback)
5. Project-level `.gmc.yaml` overrides global when present

**Config keys** (`internal/config/config.go`): `role`, `model`, `api_key`, `api_base`, `prompt_template`, `enable_emoji`.

- `prompt_template` is a **file path** to a YAML template (or `"default"` for built-in). There is no `prompts_dir` key.
- Template variables: `{{.Role}}`, `{{.Files}}`, `{{.Diff}}`

**Root command flags** agents often miss: `--timeout`, `--debug`, `-o/--output json`, stdin mode (`gmc -`).

**Diff truncation**: prompt diff is capped at 4000 bytes via smart file-priority truncation in `internal/formatter/`, not a naive string cut.

## Verification

Before claiming work is done:

```bash
make check          # fmt + lint + test — required before submission
make build          # produces ./build/gmc
make man            # after CLI surface changes
```

Targeted testing:

```bash
go test ./internal/worktree -v
go test ./cmd -v -run TestWorktree
go test -race ./...
```

Manual smoke (when changing CLI behavior):

```bash
./build/gmc --help
./build/gmc wt --help
./build/gmc version
```

## Known pitfalls

Stale assumptions that cause bad patches:

| Wrong | Correct |
|-------|---------|
| `gmc add <name>` | `gmc wt add <name>` |
| `gmc analyze` | removed; no `internal/analyzer/` |
| `docs/COBRA_GUIDE.md` | does not exist; conventions are in this file |
| Config at `~/.gmc.yaml` only | XDG `~/.config/gmc/config.yaml` is primary |
| `prompts_dir` config key | removed; `prompt_template` is a file path |
| `gmc wt promote <temp> <name>` | `promote` accepts only `<candidate>`; rename branch with `git branch -m` inside the worktree |
| Templates in `~/.gmc/prompts/` | `prompt_template` points to any YAML file path |
| Business logic in `cmd/` | move to `internal/`; keep `cmd/` as thin wiring |
| Edit `docs/man/gmc-wt-add.1` (or any `docs/man/*.1`) | edit `Short`/`Long`/`Example` in `cmd/*.go`, then `make man` |

## Development commands

```bash
make build          # ./build/gmc
make install        # copy to GOPATH/bin
make fmt            # go fmt + go mod tidy
make lint           # golangci-lint
make test           # go test ./...
make check          # fmt + lint + test
make man            # regenerate docs/man/
make clean          # remove ./build/
```

Go **1.24+**. Key deps: Cobra, Viper, `go-openai` (OpenAI-compatible API).

## Release (only when doing release work)

- Tags `v*` trigger GoReleaser ([`.goreleaser.yaml`](.goreleaser.yaml))
- Homebrew formula update: `make update-homebrew` (requires `GH_PAT`)
- Validate locally: `goreleaser check` and `goreleaser release --snapshot --clean`

Changes must stay backward-compatible unless an explicit migration path is documented.
