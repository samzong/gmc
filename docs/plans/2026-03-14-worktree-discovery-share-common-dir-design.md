# Worktree Discovery + Shared Config Design

## Goal
Make `gmc wt` discover and display all related worktrees from any repository/worktree directory while keeping `.bare` as the preferred initialization layout for `gmc wt clone` / `gmc wt init`. Also make `gmc wt share` usable in existing non-bare worktree repositories.

## Decisions

### 1. Discovery is Git-first
- Worktree discovery uses Git's shared metadata (`git worktree list --porcelain`, `git rev-parse --git-common-dir`).
- `.bare` remains the default creation/layout mode for gmc-managed repositories, but discovery no longer depends on `.bare` being present.
- `gmc wt` default output shows worktrees whenever the current directory belongs to a repo/worktree family.

### 2. Shared config lives in git common dir
- Canonical path: `<git-common-dir>/gmc-share.yml`
- Backward-compatible reads: legacy `.gmc-shared.yml` / `.gmc-shared.yaml`
- In `.bare` mode this naturally resolves under `.bare/`.
- In normal repositories this resolves under `.git/`.

### 3. Shared resource paths are normalized to worktree-relative paths
- `gmc wt share add <path>` stores a cleaned relative path.
- Paths escaping the worktree (`..`) are rejected.
- Existing legacy configs that implicitly used a source-worktree prefix continue to be tolerated during sync.

### 4. Sync targets use actual worktree paths
- Sync logic no longer assumes all worktrees live under `<root>/<name>`.
- `SyncAllSharedResources` iterates real worktree paths from Git metadata.
- This enables support for linked worktrees located anywhere on disk.

## Tradeoffs
- Canonical source resolution still prefers the main repo root, then falls back to the current worktree if needed. This keeps the change small and practical, but does not yet implement a more advanced dedicated shared-resource store.
- Display names for worktrees prefer repo-relative paths when reasonable, otherwise fall back to basename for externally located worktrees.

## Verification Plan
- Unit/integration test: `gmc wt` default output works from a non-bare linked worktree.
- Unit/integration test: shared config path resolves to git common dir in a non-bare linked worktree.
- Unit/integration test: sync copies shared resources into linked worktrees in a non-bare repo family.
- Regression test: legacy `.bare`-style shared-resource sync test remains green.
