#!/usr/bin/env bash
set -euo pipefail

DRY_RUN=false
AUTO_ADD=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run) DRY_RUN=true; shift ;;
    --auto) AUTO_ADD=true; shift ;;
    -h|--help)
      echo "Usage: wt-share-discover.sh [--dry-run] [--auto]"
      echo ""
      echo "Discover files that should be shared across worktrees."
      echo "Scans gitignored files present in the main worktree."
      echo ""
      echo "Options:"
      echo "  --dry-run   Show discoveries without adding"
      echo "  --auto      Auto-add discovered files (no confirmation)"
      exit 0
      ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

if ! command -v gmc &>/dev/null; then
  echo "Error: gmc CLI required" >&2
  exit 1
fi

SHARE_PATTERNS_COPY=(
  ".env"
  ".env.local"
  ".env.development"
  ".env.production"
  ".claude/settings.json"
  ".claude/CLAUDE.md"
  ".serena/project.yml"
)

SHARE_PATTERNS_LINK=(
  "node_modules"
  ".venv"
  "vendor"
  "__pycache__"
)

REPO_ROOT=$(git rev-parse --show-toplevel 2>/dev/null || echo "")
if [[ -z "$REPO_ROOT" ]]; then
  GIT_COMMON=$(git rev-parse --git-common-dir 2>/dev/null || echo "")
  if [[ -n "$GIT_COMMON" ]]; then
    REPO_ROOT=$(dirname "$GIT_COMMON")
  else
    echo "Error: not in a git repository" >&2
    exit 1
  fi
fi

MAIN_WT=""
while IFS= read -r line; do
  [[ -z "$line" ]] && continue
  NAME=$(echo "$line" | awk '{print $1}')
  BRANCH=$(echo "$line" | awk '{print $2}')
  if [[ "$BRANCH" == "main" || "$BRANCH" == "master" ]]; then
    MAIN_WT="$NAME"
    break
  fi
done < <(gmc wt ls 2>/dev/null | tail -n +2)

if [[ -z "$MAIN_WT" ]]; then
  echo "Error: cannot find main worktree" >&2
  exit 1
fi

if [[ "$MAIN_WT" != /* ]]; then
  MAIN_WT="$REPO_ROOT/$MAIN_WT"
fi

GIT_COMMON=$(git -C "$MAIN_WT" rev-parse --git-common-dir 2>/dev/null || echo "")
if [[ -z "$GIT_COMMON" ]]; then
  GIT_COMMON="$REPO_ROOT/.bare"
fi

EXISTING=()
if [[ -f "$GIT_COMMON/gmc-share.yml" ]]; then
  while IFS= read -r p; do
    EXISTING+=("$p")
  done < <(grep -E '^\s+path:' "$GIT_COMMON/gmc-share.yml" 2>/dev/null | sed 's/.*path:\s*//' | tr -d '"' | tr -d "'")
fi

is_already_shared() {
  local check="$1"
  for e in "${EXISTING[@]+"${EXISTING[@]}"}"; do
    [[ "$e" == "$check" ]] && return 0
  done
  return 1
}

FOUND_COPY=()
FOUND_LINK=()

for pattern in "${SHARE_PATTERNS_COPY[@]}"; do
  target="$MAIN_WT/$pattern"
  if [[ -e "$target" ]] && ! is_already_shared "$pattern"; then
    FOUND_COPY+=("$pattern")
  fi
done

for pattern in "${SHARE_PATTERNS_LINK[@]}"; do
  target="$MAIN_WT/$pattern"
  if [[ -e "$target" ]] && ! is_already_shared "$pattern"; then
    FOUND_LINK+=("$pattern")
  fi
done

if [[ ${#FOUND_COPY[@]} -eq 0 ]] && [[ ${#FOUND_LINK[@]} -eq 0 ]]; then
  echo "No new shareable files discovered."
  if [[ ${#EXISTING[@]} -gt 0 ]]; then
    echo ""
    echo "Currently shared:"
    gmc wt share ls 2>/dev/null
  fi
  exit 0
fi

echo "=== Discovered shareable files ==="
echo ""

if [[ ${#FOUND_COPY[@]} -gt 0 ]]; then
  echo "Copy strategy (isolated per worktree):"
  for f in "${FOUND_COPY[@]}"; do
    echo "  $f"
  done
fi

if [[ ${#FOUND_LINK[@]} -gt 0 ]]; then
  echo "Link strategy (shared, saves disk):"
  for f in "${FOUND_LINK[@]}"; do
    echo "  $f"
  done
fi

if [[ "$DRY_RUN" == "true" ]]; then
  echo ""
  echo "[dry-run] Would add ${#FOUND_COPY[@]} copy + ${#FOUND_LINK[@]} link resources"
  exit 0
fi

if [[ "$AUTO_ADD" == "false" ]]; then
  echo ""
  echo "Run with --auto to add these, or use gmc wt share add manually."
  exit 0
fi

cd "$MAIN_WT"

for f in "${FOUND_COPY[@]}"; do
  echo "Adding (copy): $f"
  gmc wt share add "$f" --strategy copy
done

for f in "${FOUND_LINK[@]}"; do
  echo "Adding (link): $f"
  gmc wt share add "$f" --strategy link
done

echo ""
echo "Syncing to all worktrees..."
gmc wt share sync
echo "Done."
