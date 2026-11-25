# Public contracts for v1.0.0

The v1.0.0-rc.1 audit uses the command tree and configuration readers at the release commit.
The candidate does not authorize the final v1.0.0 release.
Issue [#130](https://github.com/samzong/gmc/issues/130) stays open until final approval.

## Compatibility

Preserve existing command paths, flags, JSON field names, value types, and configuration precedence in v1.x.
New optional flags and JSON fields are compatible additions.
Consumers must ignore unknown JSON fields and must not depend on object key order.
Text tables, progress messages, error wording, absolute paths, and measured sizes are not fixed values.
JSON `status` values describe current worktree state. Consumers must tolerate new status descriptions.
Removing a public command or changing a field type requires a major release or an explicit migration.

## Root command

Run `gmc` to generate and commit a message from staged changes.
Run `gmc -` to read a diff from stdin.
Run `gmc --help` for current usage and examples.

| Flags | Type | Default | Scope |
| --- | --- | --- | --- |
| `--config` | string | Automatic file selection | All commands |
| `--debug` | bool | `false` | All commands |
| `-o`, `--output` | string | `text` | All commands, structured output where implemented |
| `-V`, `--version` | bool | `false` | Root |
| `--no-verify`, `--no-signoff`, `--dry-run` | bool | `false` | Root |
| `-a`, `--all`, `-y`, `--yes`, `-v`, `--verbose` | bool | `false` | Root |
| `--issue`, `-b` / `--branch`, `-p` / `--prompt` | string | Empty | Root |
| `--timeout` | integer seconds | `30` | Root |

`--output` accepts only `text` or `json`.
It does not convert every command into a JSON API.
The structured worktree commands are listed below.

| Exit code | Meaning |
| --- | --- |
| `0` | Success, including a cancelled interactive workflow |
| `1` | General failure, including invalid flags and arguments |
| `10` | No staged changes in the root commit workflow |
| `11` | Not a Git repository in classified commit or task failures |
| `12` | LLM failure or missing generated commit subject |

Worktree errors currently use `1`, including execution outside a Git repository.
The internal `Usage = 2` constant is not a promise that Cobra returns `2`.

## Worktree commands

`gmc wt` and `gmc worktree` address the same command group.
Without a subcommand, they list worktrees.
Read each command's `--help` for arguments and flag relationships.

| Command | Aliases | Purpose |
| --- | --- | --- |
| `add [name...]` | None | Create worktrees, or use `--pr` for a pull request |
| `list` | `ls` | List this repository, or use `-A` for home scanning |
| `remove [name...]` | `rm` | Remove worktrees, retaining branches unless `-D` is set |
| `clone <url>` | None | Create a bare clone and default worktree |
| `dup [count]` | None | Create candidates, default count `2` |
| `promote <candidate>` | None | Apply candidate changes without committing or deleting |
| `prune` | None | Prune merged worktrees and optional branches |
| `pr-review <PR_NUMBER>` | None | Create a pull request worktree |
| `init <bash\|zsh\|fish>` | None | Print shell integration |
| `switch` | None | Select a worktree through shell integration |
| `sync` | None | Fast-forward the selected base |
| `share add/remove/list/sync/discover` | `remove`: `rm`, `list`: `ls` | Manage shared resources |
| `hook add/remove/list` | `remove`: `rm`, `list`: `ls` | Manage preparation hooks |

Use the name from `wt list` or an absolute worktree path for removal.
A creation result's `name` is the requested name; it can differ from the list name in a non-bare checkout.

The `share` group without a subcommand opens interactive management.
The `hook` group without a subcommand lists hooks.

## Worktree JSON

Structured commands write JSON to stdout and progress or warnings to stderr.
List, creation, sharing, hooks, and ordinary prune results use arrays, including empty arrays.
A single-repository `wt prune --pr-aware` returns `null` when it has no entries; consumers must accept this existing exception.
With `--all`, an empty PR-aware prune result is `[]`.
List and creation tests cover the serialized field names independently of the Go struct tags.
Existing behavior tests cover list states, partial creation failures, discovery, hooks, and prune actions.

| Command | Required fields | Optional fields |
| --- | --- | --- |
| `wt`, `wt list` | Array of `name`, `path`, `branch`, `commit`, `status` strings | `repository`, `diff_base`, `review_provider`, `review_state`, `review_url` strings; `locked`, `prunable` booleans; `changed_files`, `insertions`, `deletions`, `review_number`, `allocated_bytes` integers; `issues` array |
| `wt add`, `wt add --pr`, `wt pr-review`, `wt dup` | Array of `name` string and `created` boolean | `path`, `branch`, `base`, `error` strings; `warnings` array of strings |
| `wt prune` without `--pr-aware` | Array of `name`, `branch`, `status`, `action` strings | `repository` string for home scanning |
| `wt prune --pr-aware` | Array of `name`, `branch`, `pr_state`, `action`, `reason` strings, or the empty-result exception above | `repository` string for home scanning; `pr_number` integer |
| `wt share list` | Array of `path`, `strategy`, `origin` strings and `disabled` boolean | None |
| `wt hook`, `wt hook list` | Array of `index` integer, `cmd`, `origin` strings, `disabled` boolean | `id`, `desc` strings |
| `wt share discover` | Object with `resources` and `hooks` arrays | None |

Ordinary prune uses action values such as `would-remove`; PR-aware prune uses `would_remove`.
An empty `pr_state` string means no PR state was found.

Each issue has `source`, `code`, and `message` strings.
Size issue codes are `not_found`, `permission_denied`, `overflow`, and `scan_failed`.
A size failure omits `allocated_bytes`; a measured zero remains `0`.
Diff counts appear when a diff has changes, including a zero insertion or deletion count.
Optional empty strings, false booleans, and empty arrays are omitted unless listed as required.
Creation reports every requested name on partial failure and returns a nonzero exit code.
`dup` stops at its first creation failure and preserves earlier candidates.

Each discovery resource requires `path`, `reason`, `status` strings and `size_bytes` integer.
Optional resource strings are `strategy`, `source`, `project`, `ecosystem`, `package_manager`, `configured_by`, and `target_state`.
The optional `size_limited` boolean signals a bounded size scan.
Discovery hooks use the hook schema above.

## Configuration

File selection uses the first available choice: `--config`, `GMC_CONFIG`, XDG, then legacy.
Explicit paths select that file even when the file does not exist.
XDG means `$XDG_CONFIG_HOME/gmc/config.yaml`, with `~/.config` as the default directory.
The legacy fallback is `~/.gmc.yaml`.
Project configuration is `.gmc.yaml` in the current directory, without a parent-directory search.

Nonempty `GMC_*` values override file values.
For `role`, `model`, and `enable_emoji`, a set but empty environment variable still blocks the project value.
The empty environment value then falls back to the user file or built-in default.
For example, unset `GMC_MODEL` to use the project model; `GMC_MODEL=` uses the user model or default.
An explicit `--config` or `GMC_CONFIG` skips project configuration and reports this on stderr.
Otherwise, allowed project values override the user file, which overrides built-in defaults.
Project configuration permits only `role`, `model`, and `enable_emoji`.
It cannot set `api_key`, `api_base`, `prompt_template`, or future keys without an allowlist change.
Project values do not persist into the user file.

API key precedence is `GMC_API_KEY`, OS credential storage, then the legacy file key.
Each stored credential belongs to the resolved absolute configuration file path.
Commands that do not need a key do not read credential storage.
Unavailable storage permits an existing file key with a warning.
Saving a new key requires working storage and removes file keys only after storage succeeds.
Tests use the provider's mock storage. Candidate package acceptance does not prove each operating system's native credential provider.
See [README.md](../README.md#configuration) for setup and migration.

## Repeat the audit

Run `go test ./cmd ./internal/config -run TestPublic -count=1` for the added contracts.
Run `make check` and `make build` for the repository gates.
Run the candidate package steps in the [release workflow](../website/content/docs/(developer)/release-workflow.mdx).
The tests add no provider calls and do not require a live LLM or a user's credential store.
