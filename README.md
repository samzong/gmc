# gmc - Git Message Commit

<div align="center">
  <img src="./logo.png" alt="gmc logo" width="200" />
  <br />
  <p>gmc is a CLI tool that accelerates the efficiency of Git add and commit by using LLM to generate high-quality commit messages, thereby reducing the cognitive load on developers when submitting code.</p>
  <p>
    <a href="https://github.com/samzong/gmc/releases"><img src="https://img.shields.io/github/v/release/samzong/gmc" alt="Release Version" /></a>
    <a href="https://goreportcard.com/report/github.com/samzong/gmc"><img src="https://goreportcard.com/badge/github.com/samzong/gmc" alt="go report" /></a>
    <a href="https://github.com/samzong/gmc/blob/main/LICENSE"><img src="https://img.shields.io/github/license/samzong/gmc" alt="MIT License" /></a>
  </p>
</div>

## Core Features

1. **One-command commit**: stage + generate + commit (interactive)
2. 🔥 **Smart message generation**: reads your staged diff and formats to Conventional Commits
3. **OpenAI-compatible API support**: configure API key + optional base URL (proxy/providers)
4. **Role & prompt control**: set a role, pick templates, add extra prompt context on demand
5. **Branch name generation**: create and switch to a new branch from a description (`--branch`)
6. **Auto semantic version tagging**: suggest and create annotated tags (`gmc tag`)
7. 🔥 **Worktree workflows**: manage `.bare` + worktree structure (`gmc wt`)

## Usage

### Quick start (required config)

`gmc` reads configuration from `~/.config/gmc/config.yaml` by default (override with `--config`). If a legacy `~/.gmc.yaml` exists, it is used as a fallback. On macOS/Linux, the config file is forced to permission `0600`. You can also place a `.gmc.yaml` in your project directory to override global settings on a per-repo basis.

Recommended: run the guided setup (or accept the prompt shown on first use):

```bash
gmc init
```

Manual configuration (alternative to `gmc init`):

```bash
gmc config set apibase https://your-proxy-domain.com/v1
gmc config set apikey YOUR_OPENAI_API_KEY
gmc config set model gpt-4.1-mini
```

And Configure other parameters.

```bash
gmc config set role "Backend Developer"
gmc config set enable_emoji true
gmc config set prompt_template default
gmc config set prompt_template /path/to/prompt.yaml

gmc config get
```

### Commit workflow

```bash
# Uses staged changes (git diff --cached)
gmc

# Stage all changes before generating the message
gmc -a

# Commit only specific files:
# - with -a: stage those files (and commit only those files)
# - without -a: commit only if those files are already staged
gmc -a path/to/file1 path/to/file2

# Append an issue reference (added to the end of the subject)
gmc --issue 123

# Create and switch to a new branch from a description
gmc --branch "implement user authentication"

# Add extra context/instructions to the LLM prompt
gmc --prompt "Focus on user-visible behavior, not refactors"

# Generate the message but do not run git commit
gmc --dry-run

# Skip hooks and/or DCO signoff
gmc --no-verify
gmc --no-signoff

# Auto-confirm the generated message (no prompt)
gmc --yes

# Verbose output (prints git command output)
gmc --verbose

# Provide a custom config file
gmc --config /path/to/.gmc.yaml
```

### Interactive actions

After generating a message, `gmc` prompts: `y/n/r/e`:

- `y` (or empty): commit
- `n`: cancel
- `r`: regenerate
- `e`: edit in `$EDITOR` (or `$VISUAL`, fallback to `vi`)

## Commands

### Suggest and create a semantic version tag

```bash
gmc tag
gmc tag --yes
```

`gmc tag` always runs a rule-based suggestion; if an API key is configured, it will also ask the LLM and validate/fallback safely. See `docs/auto-versioning-kep.md`.

### 🔥 Worktree management (`gmc wt`)

```bash
gmc wt
gmc wt list
gmc wt add feature-login -b main
gmc wt remove feature-login -D
gmc wt clone https://github.com/user/repo.git --name my-project
```

`gmc wt clone` creates a `.bare/` directory containing the bare repository plus a worktree for the default branch (e.g., `main/`).

#### Parallel development with dup/promote

```bash
gmc wt dup              # Create 2 worktrees for parallel AI development
gmc wt dup 3 -b dev     # Create 3 worktrees based on dev branch
gmc wt promote .dup-1 feature/best-solution  # Rename temp branch to permanent
```

This is designed for the `.bare` + worktree pattern. See `docs/auto-bare-worktree.md`.

#### Open source (fork + upstream) workflow

Clone your fork and register the upstream remote in one command:

```bash
gmc wt clone https://github.com/me/my-fork.git \
  --upstream https://github.com/org/upstream-repo.git \
  --name upstream-repo
```

Then work from the default branch worktree (usually `main/` or `master/`) and keep it synced:

```bash
cd upstream-repo/main
git fetch upstream
git merge upstream/main

# Create a feature worktree based on the updated default branch
gmc wt add feature-login  # defaults source main
```

### Shell completion

```bash
# One-off (current shell session)
source <(gmc completion bash)
source <(gmc completion zsh)

# Persistent install (recommended)
mkdir -p ~/.zsh/completions ~/.bash_completion.d
gmc completion zsh > ~/.zsh/completions/_gmc
gmc completion bash > ~/.bash_completion.d/gmc
```

For zsh, ensure `~/.zsh/completions` is in your `fpath` (e.g. in `~/.zshrc`):

```bash
fpath=(~/.zsh/completions $fpath)
autoload -Uz compinit && compinit
```

### Man pages

Man pages under `docs/man/` are generated via `make man` (from `cmd/gendoc/main.go`). Do not edit them manually.

## Prompt templates

`gmc` supports a single prompt template override. If `prompt_template` is empty or set to `default`, the built-in template is used.

Set `prompt_template` to a file path to override the built-in template:

```bash
gmc config set prompt_template /path/to/my_template.yaml
```

Create a YAML template file, for example `/path/to/my_template.yaml`:

```yaml
name: 'My Template'
description: 'My commit message format'
template: |
  As a {{.Role}}, please generate a commit message that follows the Conventional Commits specification for the following Git changes:

  Changed Files:
  {{.Files}}

  Changed Content:
  {{.Diff}}

  Commit message format requirements:
  - Use the "type(scope): description" format
  - The type must be one of: feat, fix, docs, style, refactor, perf, test, chore
  - The scope should be specific, and the description should be concise
  - Do not include issue numbers
```

### Template variables

You can use the following variables in the template:

- `{{.Role}}`: The user configured role
- `{{.Files}}`: Changed files (newline-separated)
- `{{.Diff}}`: Staged diff (truncated to 4000 characters for very large diffs)

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details
