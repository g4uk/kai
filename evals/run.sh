#!/bin/bash
# Eval runner. POLICY: every project-touching execution (agent AND checks)
# happens inside the harness-runner Docker container. This script only orchestrates.
#
# HARNESS_ALLOW_HOST=1 is an escape hatch for debugging — never for real runs.
set -u
REPO=$(git rev-parse --show-toplevel)
KIT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
RUN="$KIT_DIR/docker/claude-run.sh"
EXEC="$KIT_DIR/docker/exec.sh"

if [ "${HARNESS_ALLOW_HOST:-}" != "1" ]; then
  command -v docker >/dev/null || { echo "FATAL: docker required (policy: project runs only in Docker)."; exit 1; }
fi

RESULTS="$REPO/evals/results/$(date +%F-%H%M).md"
mkdir -p "$REPO/evals/results"
echo "# Eval run $(date -Iseconds)" | tee "$RESULTS"
PASS=0; TOTAL=0

# EDIT_ME: baseline check for every trace (runs inside the container)
BASE_CHECK="go test ./..."
# EDIT_ME: base branch (auto-detected; override if needed)
BASE_BRANCH=$(git symbolic-ref --short refs/remotes/origin/HEAD 2>/dev/null | sed 's|origin/||')
BASE_BRANCH=${BASE_BRANCH:-$(git rev-parse --abbrev-ref HEAD)}

# EVAL_LIMIT: run only the first N traces (PR sampling). Unset/0 = all.
LIMIT="${EVAL_LIMIT:-0}"
# EVAL_MAX_TURNS: trajectory gate — an agent thrashing past this many turns on a
# single trace is a quality signal in itself, independent of whether it eventually
# produced a passing diff. Output eval alone would miss this (mid-2026 whitepaper on
# agentic engineering: "a fluent output that skipped its verification steps is a more
# dangerous failure than one with a visible error" — trajectory eval catches that).
MAX_TURNS="${EVAL_MAX_TURNS:-30}"
TOTAL_COST=0

for TRACE in "$REPO"/evals/traces/*.md; do
  [ -e "$TRACE" ] || continue
  [ "$LIMIT" != "0" ] && [ "$TOTAL" -ge "$LIMIT" ] && break
  NAME=$(basename "$TRACE" .md); TOTAL=$((TOTAL+1))
  echo ">> trace $NAME (up to 5 min)..."

  # Plain clone, not a worktree: a worktree's .git file holds an absolute HOST
  # path and breaks when only the worktree is mounted into a container.
  WT="/tmp/eval-$NAME"
  rm -rf "$WT"
  git clone --quiet --branch "$BASE_BRANCH" "$REPO" "$WT"

  PROMPT=$(awk '/^## Prompt/{f=1;next}/^## /{f=0}f' "$TRACE")
  "$RUN" "$WT" "$PROMPT" "/tmp/eval-$NAME.json" || true

  # Trajectory + cost: the JSON transcript (stream-json, one object per line) has
  # already been paid for by the run above — read it instead of throwing it away.
  JSON="/tmp/eval-$NAME.json"
  TURNS=$(jq -rs 'map(select(.type=="result")) | .[0].num_turns // empty' "$JSON" 2>/dev/null)
  COST=$(jq -rs 'map(select(.type=="result")) | .[0].total_cost_usd // empty' "$JSON" 2>/dev/null)
  DURATION_MS=$(jq -rs 'map(select(.type=="result")) | .[0].duration_ms // empty' "$JSON" 2>/dev/null)
  TOOLS=$(jq -rs '[.[] | select(.type=="assistant") | .message.content[]? | select(.type=="tool_use") | .name]
                   | group_by(.) | map("\(.[0])x\(length)") | join(", ")' "$JSON" 2>/dev/null)
  echo "  - trajectory: turns=${TURNS:-?} cost=\$${COST:-?} duration=${DURATION_MS:-?}ms tools=[${TOOLS:-none}]" | tee -a "$RESULTS"
  case "${TURNS:-}" in
    ''|*[!0-9]*) : ;;
    *) TOTAL_COST=$(awk -v a="$TOTAL_COST" -v b="${COST:-0}" 'BEGIN{ print a + (b+0) }') ;;
  esac

  # No parseable result = the agent run itself failed (auth, container, CLI
  # error) before producing any trajectory. That's invisible otherwise — the
  # raw claude-run.sh output only ever went to $JSON, never to the console/CI log.
  if [ -z "${TURNS:-}" ]; then
    echo "  - RAW OUTPUT (no valid result — auth/config/container error, not a code failure):" | tee -a "$RESULTS"
    head -c 2000 "$JSON" 2>/dev/null | tee -a "$RESULTS"
    echo | tee -a "$RESULTS"
  fi

  OK=1
  # No-diff note: informational only, NOT an automatic fail. A trace whose
  # feature has already merged into $BASE_BRANCH is EXPECTED to produce no
  # diff on replay — the real signal is whether the checks below still pass,
  # not whether anything changed. Making this a hard FAIL meant every trace
  # became permanently unpassable the moment its feature merged (caught live
  # on kumite-analyzer's first accumulated-trace run: 0/2, both empty-run).
  "$EXEC" "$WT" "! git diff --quiet || ! git diff --cached --quiet || [ -n \"\$(git status --porcelain)\" ]" \
    || echo "  - NOTE: no diff (feature may already be present on $BASE_BRANCH)" | tee -a "$RESULTS"

  # Trajectory gate: too many turns on one trace means the agent was thrashing,
  # not converging — a real quality problem even if it stumbled onto a passing diff.
  case "${TURNS:-}" in
    ''|*[!0-9]*) ;;
    *) [ "$TURNS" -gt "$MAX_TURNS" ] && { OK=0; echo "  - FAIL: trajectory exceeded ${MAX_TURNS} turns (${TURNS})" | tee -a "$RESULTS"; } ;;
  esac

  # Stage the agent's output: it isn't expected to commit, but checks that use
  # git-aware tools (git grep, git diff --cached) must see its new/changed files.
  "$EXEC" "$WT" "git add -A" >/dev/null 2>&1
  "$EXEC" "$WT" "$BASE_CHECK" >/dev/null 2>&1 || { OK=0; echo "  - FAIL: base check" | tee -a "$RESULTS"; }

  # BSD/GNU-portable extraction of "- [ ] cmd: <shell>" checks (no grep -P)
  while IFS= read -r CHECK; do
    [ -z "$CHECK" ] && continue
    "$EXEC" "$WT" "$CHECK" >/dev/null 2>&1 || { OK=0; echo "  - FAIL check: $CHECK" | tee -a "$RESULTS"; }
  done < <(sed -n 's/^[[:space:]]*-[[:space:]]*\[[[:space:]]*\][[:space:]]*cmd:[[:space:]]*//p' "$TRACE")

  if [ "$OK" = 1 ]; then echo "- $NAME: PASS" | tee -a "$RESULTS"; PASS=$((PASS+1));
  else echo "- $NAME: FAIL" | tee -a "$RESULTS"; fi
  rm -rf "$WT"
done
echo "" | tee -a "$RESULTS" >/dev/null
echo "**$PASS/$TOTAL** — total cost: \$${TOTAL_COST}" | tee -a "$RESULTS"
echo "Pass rate: $PASS/$TOTAL → $RESULTS"
[ "$PASS" = "$TOTAL" ] && [ "$TOTAL" -gt 0 ]
