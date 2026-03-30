#!/usr/bin/env bash
set -euo pipefail

DRY_RUN=false
FORCE=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run) DRY_RUN=true; shift ;;
    --force|-f) FORCE=true; shift ;;
    -h|--help)
      echo "Usage: wt-cleanup.sh [--dry-run] [--force]"
      echo ""
      echo "Analyze worktrees against GitHub PR status."
      echo "Safely removes worktrees whose PRs are merged and worktree is clean."
      echo ""
      echo "Options:"
      echo "  --dry-run   Show what would be removed without removing"
      echo "  --force     Force remove even if worktree is dirty"
      exit 0
      ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

if ! command -v gh &>/dev/null; then
  echo "Error: gh CLI required" >&2
  exit 1
fi
if ! command -v gmc &>/dev/null; then
  echo "Error: gmc CLI required" >&2
  exit 1
fi

REPO=$(gh repo view --json nameWithOwner -q '.nameWithOwner' 2>/dev/null || echo "")
if [[ -z "$REPO" ]]; then
  echo "Error: not in a GitHub repo or gh not authenticated" >&2
  exit 1
fi

echo "Repository: $REPO"
echo ""

REMOVABLE=()
SKIP=()

while IFS= read -r line; do
  [[ -z "$line" ]] && continue
  NAME=$(echo "$line" | awk '{print $1}')
  BRANCH=$(echo "$line" | awk '{print $2}')
  STATUS=$(echo "$line" | awk '{print $NF}')

  [[ "$BRANCH" == "main" || "$BRANCH" == "master" || "$BRANCH" == "develop" ]] && continue

  PR_JSON=$(gh pr list --repo "$REPO" --head "$BRANCH" --state all --json number,state,headRefName --limit 1 2>/dev/null || echo "[]")
  PR_STATE=$(echo "$PR_JSON" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d[0]['state'] if d else 'NONE')" 2>/dev/null || echo "NONE")
  PR_NUM=$(echo "$PR_JSON" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d[0]['number'] if d else '')" 2>/dev/null || echo "")

  if [[ "$PR_STATE" == "MERGED" ]]; then
    if [[ "$STATUS" == "clean" ]] || [[ "$FORCE" == "true" ]]; then
      REMOVABLE+=("$NAME|$BRANCH|#$PR_NUM|$STATUS")
    else
      SKIP+=("$NAME|$BRANCH|#$PR_NUM|dirty (use --force)")
    fi
  elif [[ "$PR_STATE" == "CLOSED" ]]; then
    SKIP+=("$NAME|$BRANCH|#$PR_NUM|PR closed (not merged)")
  else
    SKIP+=("$NAME|$BRANCH|${PR_NUM:+#$PR_NUM}${PR_NUM:-no PR}|active")
  fi
done < <(gmc wt ls 2>/dev/null | tail -n +2)

if [[ ${#REMOVABLE[@]} -gt 0 ]]; then
  echo "=== Safe to remove (PR merged, worktree clean) ==="
  printf "%-30s %-30s %-8s %s\n" "WORKTREE" "BRANCH" "PR" "STATUS"
  for entry in "${REMOVABLE[@]}"; do
    IFS='|' read -r n b p s <<< "$entry"
    printf "%-30s %-30s %-8s %s\n" "$n" "$b" "$p" "$s"
  done
  echo ""

  if [[ "$DRY_RUN" == "true" ]]; then
    echo "[dry-run] Would remove ${#REMOVABLE[@]} worktree(s)"
  else
    for entry in "${REMOVABLE[@]}"; do
      IFS='|' read -r n b p s <<< "$entry"
      echo "Removing: $n (branch: $b, PR: $p)"
      RMFLAGS="-D"
      [[ "$FORCE" == "true" ]] && RMFLAGS="$RMFLAGS -f"
      gmc wt rm $RMFLAGS "$n"
    done
    echo ""
    echo "Removed ${#REMOVABLE[@]} worktree(s)"
  fi
else
  echo "No worktrees ready for cleanup."
fi

if [[ ${#SKIP[@]} -gt 0 ]]; then
  echo ""
  echo "=== Skipped ==="
  printf "%-30s %-30s %-8s %s\n" "WORKTREE" "BRANCH" "PR" "REASON"
  for entry in "${SKIP[@]}"; do
    IFS='|' read -r n b p s <<< "$entry"
    printf "%-30s %-30s %-8s %s\n" "$n" "$b" "$p" "$s"
  done
fi
