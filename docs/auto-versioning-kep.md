# KEP: Auto Versioning Based on Conventional Commits

## Summary

Add a simple `gmc tag` command that analyzes commits from last tag to HEAD and suggests the next semantic version using a hybrid approach: rule engine (reliable) + LLM (intelligent when available).

## Design Philosophy

**Single command, hybrid approach**: Rule engine ensures reliability, LLM provides intelligent analysis when available.

- **Manual control**: User runs `gmc tag` when ready to release
- **Analyzes accumulated commits**: From last tag to HEAD (bundles multiple commits)
- **Reliability first**: Rule engine always works, even without LLM
- **Intelligence when available**: LLM enhances analysis, validates and falls back if invalid

## Command

```bash
gmc tag    # Analyze commits, suggest version, create tag if confirmed
```

## Core Algorithm

### Workflow

1. Get latest tag (or v0.0.0 if none)
2. Get commits from last tag to HEAD
3. **Rule-based analysis** (always runs):
   - Parse commit types (feat, fix, etc.)
   - Detect breaking changes
   - Calculate version: MAJOR/MINOR/PATCH
4. **LLM analysis** (if API key available):
   - Send commits to LLM
   - Validate response format
   - Fallback to rules if invalid
5. Show suggestion and ask confirmation
6. Create tag if confirmed

### Rule-Based Calculation

Rule engine analyzes commits and applies SemVer rules:

- **Breaking changes** → MAJOR version bump (x+1.0.0)
- **New features (feat)** → MINOR version bump (x.y+1.0)
- **Bug fixes (fix/perf/refactor)** → PATCH version bump (x.y.z+1)
- **Docs/style/test/chore** → No version change

### LLM Integration

When API key is available:

- Send all commit messages to LLM with SemVer context
- Request response in format: `VERSION: vX.Y.Z` and `REASON: ...`
- **Validate response format** (must match `vX.Y.Z` pattern)
- If invalid format → fallback to rule-based result

## Implementation

### Required Components

1. **Git functions**: Get latest tag, get commits since tag, create tag
2. **Rule engine**: Parse commits, detect breaking changes, calculate version
3. **LLM integration**: Build prompt, call LLM, validate response
4. **Command**: `gmc tag` command with confirmation flow

## Example Output

```bash
$ gmc tag

Commits since v0.1.4 (6 total):
  - feat(emoji): commit message add emoji support
  - feat(ci): add multi-arch Docker build
  - fix(formatter): handle edge case

Suggested version: v0.2.0
Reason: Multiple new features added warrant a minor version bump

Create tag v0.2.0? [y/N]: y
Tag v0.2.0 created successfully!
```

## Configuration

Uses existing config:

- `gmc config set apikey` - Optional, rule engine works without it
- `gmc config set model` - LLM model
- `gmc config set apibase` - LLM API base URL

## Implementation Plan

1. Add git functions to get tags and commits
2. Implement rule-based version calculation
3. Add LLM integration with response validation
4. Implement `gmc tag` command with confirmation
5. Testing
