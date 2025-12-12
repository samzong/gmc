# KEP: Git Worktree Management with Bare Repository Support

## Summary

Add `gmc wt` / `gmc worktree` command to gmc for simplifying multi-branch parallel development workflows based on bare repositories (enforcing `.bare` directory) + worktrees. Complete branch creation and worktree creation in a single command, reducing cognitive load and manual steps.

## Background & Motivation

### Current Pain Points

Using bare repositories + worktrees is a best practice for multi-project management scenarios:

```
my-project/
├── .bare/           # bare repository (git clone --bare)
├── main/            # worktree: main branch
├── feature-x/       # worktree: feature-x branch
└── hotfix-y/        # worktree: hotfix-y branch
```

**Current tedious workflow to create a new worktree:**

1. `cd main/` - Enter main worktree
2. `git checkout main` - Ensure on main branch
3. `git fetch origin` - Fetch latest code
4. `git branch feature-new origin/main` - Create new branch
5. `cd ..` - Return to parent directory
6. `git worktree add feature-new feature-new` - Create worktree

**Expected simplified workflow:**

```bash
gmc wt add feature-new          # One command to complete all steps
gmc wt add feature-new -b main  # Create based on specified branch
```

### Design Goals

1. **Single command to create worktree**: Automatically complete branch creation + worktree creation
2. **Enforce `.bare` convention**: Uniformly use `.bare` directory as bare repository name
3. **Best practices encapsulation**: Provide common worktree management operations
4. **Progressive design**: Start with core functionality, gradually expand

---

## Command Design

### Basic Command Structure

```bash
gmc wt <subcommand> [options]
gmc worktree <subcommand> [options]  # Full alias
```

### Default Behavior (No Subcommand)

When executing `gmc wt`, display the current worktree list + common command help to inform users of current status and available operations:

```bash
$ gmc wt

📂 Current Worktrees:
┌───────────────┬─────────────────────────────┬──────────┐
│ Branch        │ Path                        │ Status   │
├───────────────┼─────────────────────────────┼──────────┤
│ main          │ ./main                      │ clean    │
│ feature-x     │ ./feature-x                 │ modified │
└───────────────┴─────────────────────────────┴──────────┘

💡 Common Commands:
  gmc wt add <branch>      Create new worktree with branch
  gmc wt add <branch> -b   Create based on specific branch
  gmc wt rm <name>         Remove worktree (keeps branch)
  gmc wt rm <name> -D      Remove worktree and delete branch

📍 You are here: ./feature-x (branch: feature-x)
```

### Subcommand Design

#### `gmc wt add` - Create Worktree

**Core Function**: Create a new branch and simultaneously create the corresponding worktree (branch name = worktree directory name)

```bash
# Basic usage: Create new branch and worktree (based on current HEAD)
gmc wt add <name>

# Specify base branch
gmc wt add <name> -b <base-branch>
gmc wt add <name> --base <base-branch>

# Examples
gmc wt add feature-login           # Create feature-login branch and worktree based on current HEAD
gmc wt add feature-login -b main   # Create feature-login branch and worktree based on main
gmc wt add hotfix-bug123 -b release
```

> **Design Principle**: `<name>` serves as both branch name and directory name, keeping it simple and consistent.

**Execution Logic:**

```
1. Detect repository type (bare vs regular)
2. Determine working directory location
3. Validate branch name legality
4. Fetch latest code from base branch (optional fetch)
5. Create new branch
6. Create worktree
7. Output success message and next steps
```

---

#### `gmc wt list` / `gmc wt ls` - List Worktrees

```bash
gmc wt list              # List all worktrees
gmc wt ls                # Shorthand

# Output format (same as the list portion when gmc wt has no arguments)
# ┌────────────────┬──────────────────────────────────┬──────────┐
# │ Branch         │ Path                             │ Status   │
# ├────────────────┼──────────────────────────────────┼──────────┤
# │ main           │ /path/to/project/main            │ clean    │
# │ feature-x      │ /path/to/project/feature-x       │ modified │
# │ hotfix-y       │ /path/to/project/hotfix-y        │ clean    │
# └────────────────┴──────────────────────────────────┴──────────┘
```

---

#### `gmc wt remove` / `gmc wt rm` - Remove Worktree

**Default Behavior**: Follows `git worktree remove` semantics, only deletes the worktree directory, **preserves the branch**.

```bash
gmc wt remove <name>     # Remove worktree, keep branch (default)
gmc wt rm <name>         # Shorthand

# Delete branch at the same time (explicit specification)
gmc wt rm <name> --delete-branch
gmc wt rm <name> -D

# Force remove (ignore dirty working tree)
gmc wt rm <name> --force
gmc wt rm <name> -f
```

> **Design Principle**: Consistent with `git worktree remove`, preserves branch by default. Deleting branch requires explicitly specifying the `-D` option to prevent accidental deletion.

---

#### `gmc wt clone` - Clone Remote Repository as Bare + Worktree Structure

**Core Function**: Clone a remote repository and automatically set up as `.bare` + worktree structure, supporting fork workflows.

```bash
# Basic usage: Clone repository
gmc wt clone <url>
gmc wt clone <url> --name <project-name>

# Fork workflow: Specify upstream repository (open source contribution scenario)
gmc wt clone <fork-url> --upstream <upstream-url>

# Example: Contributing to open source project
gmc wt clone https://github.com/samzong/daocloud-docs.git \
    --upstream https://github.com/daocloud/daocloud-docs.git
```

**Execution Steps:**

```
1. Parse URL, extract project name
2. git clone --bare <url> <name>/.bare
3. Configure .bare/config:
   - Set core.bare = false
   - Set core.worktree to point to parent directory
   - Configure remote.origin.fetch
4. If --upstream specified, add upstream remote
5. Create main worktree
6. Output success message and usage guide
```

**Generated Directory Structure:**

```
daocloud-docs/
├── .bare/                    # Bare repository
│   ├── config                # origin + upstream configuration
│   ├── HEAD
│   └── ...
└── main/                     # main branch worktree
    ├── .git                  # Points to ../.bare
    └── ...                   # Project files
```

**Remote Configuration (when using --upstream):**

```bash
# origin = your fork (for push)
git remote -v
# origin    https://github.com/samzong/daocloud-docs.git (fetch)
# origin    https://github.com/samzong/daocloud-docs.git (push)
# upstream  https://github.com/daocloud/daocloud-docs.git (fetch)
# upstream  https://github.com/daocloud/daocloud-docs.git (push)
```

---

#### `gmc wt init` - Convert Existing Repository to Bare + Worktree Structure

**Core Function**: Convert an existing regular git repository to `.bare` + worktree structure.

```bash
# Execute in existing repository directory
cd my-existing-repo
gmc wt init
```

**Execution Steps:**

```
1. Detect if current directory is a git repository
2. Backup .git directory as .bare
3. Move project files to main/ directory
4. Configure .bare/config
5. Create main worktree link
6. Output success message
```

> **Convention Constraint**: All projects managed by gmc wt must use `.bare` as the bare repository directory name for consistency.

---

## Technical Implementation

### Directory Structure

```
gmc/
├── cmd/
│   └── worktree.go              # [NEW] worktree command definition
└── internal/
    └── worktree/                # [NEW] worktree business logic
        ├── worktree.go          # Core functionality implementation
        ├── bare.go              # Bare repository utilities
        └── worktree_test.go     # Unit tests
```

### Core Function Design

```go
package worktree

// DetectRepositoryType detects current repository type
func DetectRepositoryType() (RepoType, error)

// RepoType repository type
type RepoType int
const (
    RepoTypeNormal RepoType = iota  // Normal git repository
    RepoTypeBare                     // Bare repository
    RepoTypeWorktree                 // Worktree directory
)

// WorktreeInfo worktree information
type WorktreeInfo struct {
    Path      string
    Branch    string
    Commit    string
    IsPrunable bool
    IsLocked   bool
}

// List lists all worktrees
func List() ([]WorktreeInfo, error)

// Add creates new worktree
func Add(branchName string, opts AddOptions) error

type AddOptions struct {
    BaseBranch string  // Base branch
    Directory  string  // Target directory
    Track      string  // Remote branch to track
    Fetch      bool    // Whether to fetch first
}

// Remove removes worktree
func Remove(name string, opts RemoveOptions) error

type RemoveOptions struct {
    Force        bool
    DeleteBranch bool
}

// GetWorktreeRoot gets worktree root directory
func GetWorktreeRoot() (string, error)
```

### Key Git Commands

```bash
# List worktrees
git worktree list --porcelain

# Add worktree (using existing branch)
git worktree add <path> <branch>

# Add worktree (creating new branch)
git worktree add -b <new-branch> <path> <start-point>

# Remove worktree
git worktree remove <path>
git worktree remove --force <path>

# Detect bare repository
git rev-parse --is-bare-repository
```

---

## User Flow Examples

### Typical Workflow

```bash
# 1. Clone project (simple scenario)
gmc wt clone https://github.com/user/my-project.git
cd my-project/

# 2. Start new feature development
gmc wt add feature-login
cd feature-login/

# ... development work ...
gmc   # Use gmc to commit

# 3. Check current status
gmc wt

# 4. Cleanup after feature completion
gmc wt rm feature-login      # Keep branch
gmc wt rm feature-login -D   # Or delete branch as well
```

### Open Source Contribution Workflow (Fork + Upstream)

```bash
# 1. Clone fork and configure upstream
cd ~/git/daocloud
gmc wt clone https://github.com/samzong/daocloud-docs.git \
    --upstream https://github.com/daocloud/daocloud-docs.git

# Created structure:
#   daocloud-docs/.bare/    # origin=samzong, upstream=daocloud
#   daocloud-docs/main/

# 2. Start new feature
cd daocloud-docs
gmc wt add fix-typo
cd fix-typo/

# 3. Sync with upstream latest code
git fetch upstream
git rebase upstream/main

# 4. Develop and commit
vim docs/guide.md
gmc

# 5. Push to fork and create PR
git push origin fix-typo
# Then create PR on GitHub

# 6. Cleanup after PR is merged
gmc wt rm fix-typo -D
```

### Handling Urgent Fixes

```bash
# Currently in feature-login development
# Need to urgently fix production bug

# 1. No need to stash, directly create hotfix worktree
gmc wt add hotfix-bug123 -b v0.2.0
cd ../hotfix-bug123/

# 2. Fix and commit
vim fix.go
gmc

# 3. Push and return to feature development
git push origin hotfix-bug123
cd ../feature-login/
# Continue development...

# 4. Cleanup hotfix (keep branch for later cherry-pick or PR)
gmc wt rm hotfix-bug123
# If branch is already merged, can delete together
gmc wt rm hotfix-bug123 -D
```

---

## Implementation Phases

### Phase 1: Core Functionality (MVP)

- [ ] `gmc wt` - No arguments displays worktree list + common command help
- [ ] `gmc wt clone <url> [--upstream <url>]` - Clone as bare + worktree structure
- [ ] `gmc wt add <branch> [-b base]` - Create worktree
- [ ] `gmc wt list` / `gmc wt ls` - List worktrees
- [ ] `gmc wt remove <name>` - Remove worktree (keep branch)
- [ ] Enforce `.bare` directory detection and validation
- [ ] Basic error handling and friendly prompts

### Phase 2: Enhanced Features

- [ ] `gmc wt init` - Convert existing repository to bare + worktree structure

---

## References

- [Git Worktree Documentation](https://git-scm.com/docs/git-worktree)
- [Bare Repository + Worktree Best Practices](https://morgan.cugeez.com/blog/worktrees-with-bare-repo/)

---

## Design Decisions

| Question             | Decision               | Reason                                                                 |
| -------------------- | ---------------------- | ---------------------------------------------------------------------- |
| Command name         | `gmc wt`               | Concise and memorable, `wt` = worktree abbreviation                    |
| Directory structure  | Enforce `.bare`        | Unified convention, easy to identify and manage                        |
| Delete behavior      | Keep branch by default | Follows `git worktree remove` semantics, prevents accidental deletion  |
| No-argument behavior | Show list + help       | Helps users quickly understand current status and available operations |
