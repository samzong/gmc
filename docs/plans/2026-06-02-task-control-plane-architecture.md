# Task Control Plane Architecture

## Status

Proposed design for issue [#76](https://github.com/samzong/gmc/issues/76).

This document defines the technical architecture for evolving gmc from a
worktree helper into a local-first control plane for human-supervised parallel
AI coding tasks.

Terminology delta from [#76](https://github.com/samzong/gmc/issues/76):

- Issue **Todo** → this doc **Task** (durable unit of work, not a session).
- Issue **Workflow / Stage** → **hardcoded workflow knobs in v1**; individual
  steps are **Runs** with types such as `agent-session` and `command-check`.
- Issue sketch `.gmc/tasks/*.yaml` → repo-family ledger under
  `<git-common-dir>/gmc-tasks/` (see Storage); optional mirrored task files in
  a worktree are not the source of truth.

This is an intentional refinement of the RFC, not an undocumented drift.

## Problem

gmc already solves the low-level container problem for parallel AI coding:
multiple isolated git worktrees can exist side by side, shared resources can be
synced, pull requests can be checked out for review, and a winning candidate can
be promoted back into the parent worktree.

The next bottleneck is task orchestration. Today the human operator manually
switches across IDEs, terminals, agent tools, worktrees, and GitHub pages. That
does not scale: concurrency stays low, task state lives in the operator's head,
and the link between input, worktree, agent session, verification, review, and
final PR is easy to lose.

## Goal

Make gmc the local-first, git-native control plane for human-supervised
parallel AI coding.

The control plane should:

- add a task from a GitHub issue, local note, or plain prompt;
- start one or more isolated attempts for that task;
- run interactive agent sessions and non-interactive commands in the right
  worktree;
- track state, logs, artifacts, and human intervention points;
- let a human attach to a live session without reconstructing context;
- record review and verification evidence before work is promoted or turned
  into a PR;
- garbage-collect runtime resources conservatively without losing useful work.

## Non-goals for v1

- No hosted service.
- No team/multi-tenant collaboration model.
- No Web dashboard.
- No custom workflow DSL.
- No generic project-management replacement.
- No attempt to hide git worktrees, branches, diffs, or PRs behind a separate
  product model.

These can be revisited after the task/session ledger is proven in real use.

## Existing gmc Assets

The design should build on current gmc behavior instead of bypassing it:

- `gmc wt dup --task` already fans out candidate worktrees and copies task
  context files.
- `gmc wt promote` already expresses the "pick the winning candidate" operation.
- `gmc wt pr-review` already creates a worktree from a review target.
- `gmc wt list --output json` already exposes machine-readable worktree state.
- `--pr` and `--diff-base` already enrich worktree listings with review and diff
  information.
- `GMC_DIRECTIVE_FILE` already gives gmc a way to make parent-shell navigation
  ergonomic.
- The repository already has terminal UI dependencies through `huh` and
  Charmbracelet libraries.

The missing layer is not another worktree command. It is a first-class
task/session ledger.

## Core Model

The model has three primary levels:

```text
Task
  Attempt
    Run
```

### Task

A task is the durable unit of work and the human-facing management process. It
is not an agent session.

Examples:

- "Fix GitHub issue #123."
- "Review PR #456."
- "Try three approaches for this refactor and promote the best one."

A task owns:

- source input;
- current high-level state;
- context files and links;
- attempts;
- event history;
- final artifacts such as PR URL, merged commit, or archive reason.

### Attempt

An attempt is one solution path for a task. It usually owns one worktree and one
branch.

One task can have one attempt:

```text
issue -> codex attempt -> PR
```

One task can also have multiple parallel attempts:

```text
issue
  -> codex attempt
  -> claude attempt
  -> opencode attempt
  -> human compares attempts
  -> promote winner
```

An attempt owns:

- worktree path;
- branch name;
- agent/model defaults;
- runs;
- diff/commit/PR artifacts;
- attempt-level state.

### Run

A run is one executable step inside an attempt. Not every run is an interactive
agent session.

Run types:

```text
agent-session
agent-review
command-check
command-fix
human-attach
```

Examples:

```text
codex                         -> agent-session
claude                        -> agent-session
opencode                      -> agent-session
codex review --uncommitted    -> agent-review
pnpm check                    -> command-check
make test                     -> command-check
go test ./...                 -> command-check
```

This distinction matters. tmux is useful for interactive sessions, but headless
review and verification commands should be modeled as first-class runs with
logs, exit codes, and artifacts.

## State Model

Task state should answer one question: what should happen next, and who owns
that next action?

It should not mirror every low-level process detail. Low-level process state
belongs to runs and attempts.

### Task States

```text
intake
planning
running
needs-human
reviewing
verifying
ready-for-pr
pr-open
done
archived
```

State meanings:

- `intake`: the task exists, but no attempt has started.
- `planning`: context is being gathered or an execution plan is being prepared.
- `running`: at least one attempt is actively producing work.
- `needs-human`: automation is blocked or a human gate is required.
- `reviewing`: produced work is being reviewed by a human or review run.
- `verifying`: checks, tests, or policy gates are running.
- `ready-for-pr`: enough evidence exists to open or update a PR.
- `pr-open`: a PR exists and the task is waiting for review, CI, or merge.
- `done`: the task reached its intended outcome.
- `archived`: the task was abandoned, superseded, or intentionally removed from
  the active queue.

### Attempt States

```text
created
running
waiting-human
human-attached
done
failed
lost
promoted
archived
```

Attempt state describes the health of one solution path. A failed attempt does
not necessarily fail the whole task if other attempts remain viable.

- `human-attached`: an operator is attached to the attempt session; GC and
  automatic stage advancement pause until detach.

### Run States

```text
queued
running
waiting-human
passed
failed
cancelled
lost
archived
```

Run state is where process-level facts live: exit code, tmux pane death, command
failure, missing runtime, timeout, or manual cancellation.

## State Progression

Task progression is derived from task events, attempt state, and run evidence.

Typical single-attempt flow:

```text
intake
  -> planning
  -> running
  -> reviewing
  -> verifying
  -> ready-for-pr
  -> pr-open
  -> done
```

Human intervention flow:

```text
running
  -> needs-human
  -> running
```

Parallel attempt flow:

```text
running
  -> reviewing
  -> ready-for-pr
```

where `reviewing` includes comparing attempts and selecting a winner.

Rule of thumb:

- `pnpm check` failing is a failed run, not a failed task.
- an agent writing a bad patch is a failed attempt, not a failed task.
- a dead tmux pane is a runtime event, not by itself a task conclusion.
- a task is only `done` after the intended external outcome is reached.

### Multi-attempt task aggregation

Phase 1 assumes one active attempt per task. Phase 2 adds explicit rules:

- Task stays `running` while any attempt is `created`, `running`, or
  `human-attached`.
- Task moves to `needs-human` when every active attempt is `waiting-human` or
  `human-attached` and no headless verification run is in progress.
- A single `failed` or `lost` attempt does not change the task if another
  attempt remains `running` or `created`.
- Task moves to `reviewing` when all non-archived attempts are terminal (`done`,
  `failed`, `lost`, or `promoted`) or ready for comparison, and at least one
  attempt has reviewable artifacts.
- Task is `done` only after the chosen attempt reaches the external outcome (PR
  merged, issue closed, or explicit archive reason).

## Storage

gmc should own a durable local ledger. The ledger is the source of truth. tmux,
processes, worktrees, and agent-native sessions are external resources that must
be reconciled against the ledger.

Proposed repo-family storage:

```text
<git-common-dir>/gmc-tasks/
  tasks/
    <task-id>/
      task.yaml
      events.jsonl
      attempts/
        <attempt-id>.yaml
      runs/
        <run-id>.yaml
      logs/
        <run-id>.log
      artifacts/
        ...
```

Using the git common dir follows the existing `gmc-share.yml` direction: task
state belongs to the repository/worktree family, not to one linked worktree.

See also:
[worktree discovery + shared config design](2026-03-14-worktree-discovery-share-common-dir-design.md)
for how `<git-common-dir>` paths, share config, and worktree discovery fit
together in the same repo-family model.

Open question: whether selected task metadata should optionally be copied into
the visible worktree as a human-readable task file. That is useful for agents,
but it should not become the primary state store.

## Event Log

`events.jsonl` is append-only. It records facts that explain how state changed.

Example event types:

```text
task.created
attempt.created
run.started
run.completed
run.failed
session.attached
session.detached
review.findings_recorded
verification.passed
verification.failed
attempt.promoted
pr.opened
task.done
task.archived
gc.runtime_removed
gc.orphan_detected
```

The YAML files are snapshots when this design is implemented. The event log is
the audit trail.

## Execution Architecture

Execution is split into runtimes and adapters.

```text
Task engine
  -> Runtime adapter
  -> Agent or command adapter
```

### Runtime Adapters

Runtime adapters own how a run is executed.

```text
tmux-runtime
headless-runtime
native-agent-runtime
pty-runtime
```

#### tmux-runtime

Use for long-lived interactive terminal sessions that a human may attach to.

Best for:

- `codex`;
- `claude`;
- `opencode`;
- other terminal agents that expect a TTY.

tmux is the v1 runtime because it is mature, inspectable, and already solves
attach/detach.

tmux is not the source of truth.

#### headless-runtime

Use for non-interactive commands.

Best for:

- `codex review --uncommitted`;
- `pnpm check`;
- `make test`;
- `go test ./...`;
- formatters and lint fixes.

The headless runtime records command, cwd, env, start/end time, exit code,
stdout, stderr, and artifact summary.

#### native-agent-runtime

Some tools expose their own session model. For example, an agent may support
native resume, export, background sessions, or server APIs.

These native features are adapter enhancements, not the primary gmc model.

#### pty-runtime

Direct PTY ownership can provide tighter TUI/Web integration later, but it is
not a v1 requirement. It adds terminal lifecycle complexity that tmux already
handles.

### Agent Adapters

Agent adapters map gmc's model to each tool's CLI.

```text
AgentAdapter
  detect
  buildInteractiveCommand
  buildReviewCommand
  attach
  resume
  inspectStatus
  extractArtifacts
```

Initial adapters:

- `codex`;
- `claude`;
- `opencode`;
- `custom`.

Agent adapters should handle model selection, command overrides, workspace
path, approval/sandbox flags, resume/fork behavior, and any agent-specific
artifact extraction.

### Command Adapter

The generic command adapter runs ordinary commands in a worktree.

```text
CommandAdapter
  run
  streamLogs
  classifyExit
  extractArtifacts
```

This keeps checks and scripts first-class without turning every command into an
agent session.

## Human Attach

Human intervention is attach/resume over an existing task context.

Commands should make the operator land in the right place with enough context:

```text
gmc task attach <task-id> --attempt <attempt-id>
gmc task detach <task-id> --attempt <attempt-id>
gmc task resume <task-id> --attempt <attempt-id>
```

Attach should:

- mark the attempt as `human-attached`;
- set the task to `needs-human` when automated progression must pause until
  detach;
- prevent automatic GC and automatic stage advancement while attached;
- change into the attempt worktree when shell integration is active;
- attach to the runtime session when possible;
- show task source, attempt id, run id, last event, diff summary, and blocking
  reason.

The user should not need to reconstruct which worktree, branch, agent, model,
or context file belongs to the task.

## Garbage Collection

GC must be conservative. The product is allowed to leave resources behind before
it is allowed to delete useful work.

Proposed commands:

```text
gmc task gc --dry-run
gmc task gc --sessions
gmc task gc --worktrees
gmc task gc --branches
gmc task gc --orphans
gmc task gc --archive --older-than 30d
gmc task rm <task-id> --archive
gmc task rm <task-id> --force
```

Rules:

- Default GC output is a dry run.
- Never delete a session with an attached human.
- Never delete a running, `waiting-human`, or `human-attached` session.
- Never delete a dirty worktree by default.
- Never delete an unpushed branch by default.
- Never delete an attempt with an open PR by default.
- Keep logs and event history longer than runtime resources.
- If the ledger exists but the runtime is gone, mark the run or attempt `lost`
  or derive a terminal state after artifact inspection.
- If a runtime exists but the ledger is missing, mark it unmanaged and offer
  import or explicit kill.

tmux options such as automatic destruction after detach are too blunt for this
product. Detach is normal. gmc should own cleanup policy.

## Reconciliation

The ledger must be reconciled with external state:

- tmux sessions and panes;
- live process IDs;
- worktree existence and git status;
- branch existence and upstream status;
- PR existence and state;
- agent-native session metadata when available.

Reconciliation should be explicit in v1:

```text
gmc task refresh
gmc task list --refresh
gmc task gc --dry-run
```

A daemon can automate this later.

## CLI MVP

Phase 1 should prove the task/session ledger without building a dashboard.

Candidate commands:

```text
gmc task add <issue-or-text>
gmc task list
gmc task show <task-id>
gmc task start <task-id> --agent <agent> --model <model>
gmc task run <task-id> --attempt <attempt-id> -- <command>
gmc task attach <task-id> --attempt <attempt-id>
gmc task gc --dry-run
```

Expected first complete chain:

```text
add task
start worktree
start codex in tmux
run codex review --uncommitted as a headless review run
run pnpm check as a headless verification run
record logs and exit codes
attach to the live session
show gc dry-run output
```

## Workflow Configuration

Do not build a workflow DSL in v1.

The early workflow should be hardcoded and backed by a few configuration knobs:

```text
default agent
default model
review command
verification command
human gates
```

Reason: the primitives are not proven yet. A workflow DSL would turn the first
implementation into a small workflow engine before gmc has proven its task,
attempt, run, runtime, and GC model.

Evolution path:

```text
v1: hardcoded workflow with configuration knobs
v2: declarative workflow config referencing existing primitives
v3: conditional workflow DSL, if real usage proves it is needed
```

If v2 happens, a workflow file should only reference gmc primitives:

```yaml
name: default
stages:
  - id: coding
    run:
      type: agent-session
      agent: codex
      command: codex --dangerously-bypass-approvals-and-sandbox
  - id: review
    run:
      type: agent-review
      command: codex review --uncommitted
  - id: verify
    run:
      type: command-check
      command: pnpm check
  - id: pr
    gate: human
```

No custom scripting DSL should be introduced until this simpler shape fails.

## TUI and Web

A board is a client, not the core architecture.

Phase order:

```text
ledger + CLI
  -> attempt fanout and review command
  -> TUI board
  -> daemon/API
  -> Web dashboard
```

The TUI should read the same state as `gmc task list --output json`.

The Web dashboard should wait until the ledger and state model survive local
usage. Starting with Web would hide architecture mistakes behind UI work.

## Security and Safety

The task control plane can run commands and agents that modify repositories.
Default behavior must be explicit and inspectable.

Required safety rules:

- command text is recorded before execution;
- cwd/worktree is recorded;
- env profile is recorded without dumping secrets;
- destructive cleanup requires force;
- PR creation is a human gate in v1;
- dirty worktree deletion is never default;
- unpushed branch deletion is never default;
- automatic agent execution should inherit each agent's approval/sandbox model
  unless the user explicitly configures otherwise.

## Implementation Phases

### Phase 1: Ledger and Local Execution

Deliver:

- task directory layout;
- task/attempt/run YAML snapshots;
- append-only event log;
- add/list/show commands;
- tmux runtime adapter for interactive sessions;
- headless runtime for commands;
- attach command;
- gc dry-run.

Do not deliver:

- Web UI;
- daemon;
- workflow DSL;
- hosted execution.

### Phase 2: Parallel Attempts and Review

Deliver:

- task fanout across multiple attempts;
- agent/model selection per attempt;
- review runs;
- verification runs;
- compare/promote flow;
- PR state recording.

### Phase 3: Board

Deliver:

- TUI board over the ledger;
- filters for running, needs-human, reviewing, verifying, and ready-for-pr;
- attach/resume actions from the board.

### Phase 4: Daemon and API

Deliver only if earlier phases prove the model:

- background reconciler;
- live status API;
- notifications;
- Web dashboard;
- optional mobile access.

## Open Questions

- Should task state live only under the git common dir, or should there also be
  a global index for cross-repository boards?
- What is the stable task id format: issue number, slug, timestamp, or generated
  id?
- How much agent transcript should be captured by gmc versus left in the
  agent-native store?
- Which review result schema is useful enough for v1 without overfitting to one
  agent?
- Should `gmc wt dup --task` become an implementation detail of `gmc task fanout`,
  or remain a lower-level command with shared internals?
- What is the minimum useful `ready-for-pr` evidence policy?

## Decision Summary

- Adopt `Task -> Attempt -> Run` as the core model.
- Treat task state as a human-facing management state, not a process state.
- Use tmux as the v1 runtime for attachable interactive sessions.
- Use direct headless execution for review and verification commands.
- Keep gmc's ledger as the source of truth.
- Keep git worktrees as the code sandbox.
- Keep agent tools behind adapters.
- Defer workflow DSL, daemon, Web UI, and hosted execution.
