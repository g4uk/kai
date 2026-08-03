#!/bin/bash
# Eval runner. POLICY: the agent's own run ALWAYS happens inside the
# harness-runner Docker container — no exceptions, no environment check.
# Checks (base check + trace cmd:, human-approved via /harness:retro before they're
# ever committed) run in that same sandbox by default, but directly on the
# host when HARNESS_EVAL_CHECKS_HOST=1 (see run_check below) — for a CI
# environment that's already a fresh, isolated, single-job VM with its own
# native Docker daemon, where nesting harness-runner around checks only
# breaks docker/docker-compose checks for no safety benefit (harness-runner
# itself has no docker CLI/socket). This script has no opinion on which CI
# vendor sets that flag — see ci/harness-evals.yml for the GitHub wiring.
# This script only orchestrates.
#
# HARNESS_ALLOW_HOST=1 is an escape hatch for debugging — never for real runs.
set -u
REPO=$(git rev-parse --show-toplevel)
KIT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

# Self-adjusting gate: skip entirely — before requiring docker, before any
# API call — if there aren't enough traces yet for a baseline worth
# protecting. Default 0 = always run (a human invoking this manually always
# means it); CI sets a real threshold (see ci/harness-evals.yml's
# EVAL_MIN_TRACES) so its triggers can stay on from day one without ever
# needing a human to hand-edit the workflow later as traces accumulate.
MIN_TRACES="${EVAL_MIN_TRACES:-0}"
TRACE_COUNT=$(find "$REPO/harness/evals/traces" -maxdepth 1 -name '*.md' 2>/dev/null | wc -l | tr -d ' ')
if [ "$TRACE_COUNT" -lt "$MIN_TRACES" ]; then
  echo "Only $TRACE_COUNT trace(s) in harness/evals/traces/ — need $MIN_TRACES for a baseline"
  echo "(scenarios/greenfield.md Stage 3). Skipping; no docker required, no API calls made."
  exit 0
fi

RUN="$KIT_DIR/docker/claude-run.sh"
EXEC="$KIT_DIR/docker/exec.sh"

if [ "${HARNESS_ALLOW_HOST:-}" != "1" ]; then
  command -v docker >/dev/null || { echo "FATAL: docker required (policy: project runs only in Docker)."; exit 1; }
fi

# Checks (base check + trace cmd:) are human-approved commands (via /harness:retro's
# approval gate before they're ever committed) — not agent-authored arbitrary
# code. The agent's own run ($RUN, above) ALWAYS stays in the harness-runner
# sandbox regardless of environment; that boundary is unrelated to this one.
# HARNESS_EVAL_CHECKS_HOST=1 is an explicit opt-in for a CI environment that's
# already a fresh, single-job, isolated VM with its own native Docker daemon —
# nesting harness-runner around checks there only breaks anything needing
# docker/docker compose (harness-runner deliberately ships no docker
# CLI/socket; that's the agent sandbox's edge, not meant to be punched
# through just to make checks work). Unset by default: stay fully sandboxed
# — a plain dev machine is shared/persistent, not a throwaway CI VM, and this
# script doesn't guess vendor/environment on its own.
run_check() {
  local dir="$1"; shift
  if [ "${HARNESS_EVAL_CHECKS_HOST:-}" = "1" ]; then
    ( cd "$dir" && bash -c "$*" )
  else
    "$EXEC" "$dir" "$@"
  fi
}

RESULTS="$REPO/harness/evals/results/$(date +%F-%H%M).md"
mkdir -p "$REPO/harness/evals/results"
echo "# Eval run $(date -Iseconds)" | tee "$RESULTS"
PASS=0; TOTAL=0

# EDIT_ME: baseline check for every trace (runs inside the container)
BASE_CHECK="go test ./..."
# EDIT_ME: base branch (auto-detected; override if needed)
BASE_BRANCH=$(git symbolic-ref --short refs/remotes/origin/HEAD 2>/dev/null | sed 's|origin/||')
BASE_BRANCH=${BASE_BRANCH:-$(git rev-parse --abbrev-ref HEAD)}

# EVAL_LIMIT: run only the first N traces (PR sampling). Unset/0 = all.
LIMIT="${EVAL_LIMIT:-0}"
# EVAL_TRACES: comma/space-separated name substrings — run only traces that
# match one of them, e.g. EVAL_TRACES="007,008,009" while debugging a
# specific failure, instead of paying to re-run the whole accumulated set
# every time. Unset = no filter (every trace is a candidate, same as before).
TRACES_FILTER="${EVAL_TRACES:-}"
# EVAL_MAX_TURNS: trajectory gate — an agent thrashing past this many turns on a
# single trace is a quality signal in itself, independent of whether it eventually
# produced a passing diff. Output eval alone would miss this (mid-2026 whitepaper on
# agentic engineering: "a fluent output that skipped its verification steps is a more
# dangerous failure than one with a visible error" — trajectory eval catches that).
MAX_TURNS="${EVAL_MAX_TURNS:-30}"
TOTAL_COST=0

for TRACE in "$REPO"/harness/evals/traces/*.md; do
  [ -e "$TRACE" ] || continue
  NAME=$(basename "$TRACE" .md)
  if [ -n "$TRACES_FILTER" ]; then
    MATCHED=0
    NUM_PREFIX="${NAME%%-*}"
    IFS=', ' read -ra WANT <<< "$TRACES_FILTER"
    for w in "${WANT[@]}"; do
      [ -z "$w" ] && continue
      # exact match on the number prefix (e.g. "007") or the full name
      # (e.g. "007-session-revalidation") — not a bare substring, which
      # would let "007" wrongly match a future "0072-..." or "1007-...".
      if [ "$w" = "$NUM_PREFIX" ] || [ "$w" = "$NAME" ]; then MATCHED=1; break; fi
    done
    [ "$MATCHED" = 0 ] && continue
  fi
  [ "$LIMIT" != "0" ] && [ "$TOTAL" -ge "$LIMIT" ] && break
  TOTAL=$((TOTAL+1))
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

  # A PARSEABLE result can still be an error (auth failure, rate limit, etc.)
  # — e.g. "is_error":true, num_turns:1, result:"Not logged in · Please
  # run /login". That's a completely different problem than a legitimate
  # empty diff (a merged feature with nothing left to do), but downstream
  # they look identical: no diff, base check fails on missing code. Without
  # this, an auth failure reads as a code/test problem, sending debugging
  # down the wrong layer entirely (caught running this repo's own
  # tests/runner-smoke.sh without ANTHROPIC_API_KEY set locally).
  IS_ERROR=$(jq -rs 'map(select(.type=="result")) | .[0].is_error // false' "$JSON" 2>/dev/null)
  if [ "$IS_ERROR" = "true" ]; then
    RESULT_TEXT=$(jq -rs 'map(select(.type=="result")) | .[0].result // empty' "$JSON" 2>/dev/null)
    echo "  - AGENT ERROR (is_error=true, not a code/test failure): ${RESULT_TEXT:-<no message>}" | tee -a "$RESULTS"
  fi

  OK=1
  # No-diff note: informational only, NOT an automatic fail. A trace whose
  # feature has already merged into $BASE_BRANCH is EXPECTED to produce no
  # diff on replay — the real signal is whether the checks below still pass,
  # not whether anything changed. Making this a hard FAIL meant every trace
  # became permanently unpassable the moment its feature merged (caught live
  # on kumite-analyzer's first accumulated-trace run: 0/2, both empty-run).
  run_check "$WT" "! git diff --quiet || ! git diff --cached --quiet || [ -n \"\$(git status --porcelain)\" ]" \
    || echo "  - NOTE: no diff (feature may already be present on $BASE_BRANCH)" | tee -a "$RESULTS"

  # Trajectory gate: too many turns on one trace means the agent was thrashing,
  # not converging — a real quality problem even if it stumbled onto a passing diff.
  case "${TURNS:-}" in
    ''|*[!0-9]*) ;;
    *) [ "$TURNS" -gt "$MAX_TURNS" ] && { OK=0; echo "  - FAIL: trajectory exceeded ${MAX_TURNS} turns (${TURNS})" | tee -a "$RESULTS"; } ;;
  esac

  # Stage the agent's output: it isn't expected to commit, but checks that use
  # git-aware tools (git grep, git diff --cached) must see its new/changed files.
  run_check "$WT" "git add -A" >/dev/null 2>&1
  # A FAIL with no diagnostic is a dead end — a docker-compose/mysql check
  # that fails in CI gives no way to tell "build error" from "migration
  # never ran" from "port conflict" apart, at exactly the point someone
  # needs to know. Same reasoning as the agent's own RAW OUTPUT surfacing
  # above, applied to check output: capture it, print the tail (where a
  # build/test failure's actual reason usually is) on FAIL, capped so one
  # noisy check can't drown the results file.
  if ! CHECK_OUT=$(run_check "$WT" "$BASE_CHECK" 2>&1); then
    OK=0
    echo "  - FAIL: base check" | tee -a "$RESULTS"
    echo "$CHECK_OUT" | tail -c 2000 | tee -a "$RESULTS"
  fi

  # BSD/GNU-portable extraction of "- [ ] cmd: <shell>" checks (no grep -P)
  while IFS= read -r CHECK; do
    [ -z "$CHECK" ] && continue
    if ! CHECK_OUT=$(run_check "$WT" "$CHECK" 2>&1); then
      OK=0
      echo "  - FAIL check: $CHECK" | tee -a "$RESULTS"
      echo "$CHECK_OUT" | tail -c 2000 | tee -a "$RESULTS"
    fi
  done < <(sed -n 's/^[[:space:]]*-[[:space:]]*\[[[:space:]]*\][[:space:]]*cmd:[[:space:]]*//p' "$TRACE")

  if [ "$OK" = 1 ]; then echo "- $NAME: PASS" | tee -a "$RESULTS"; PASS=$((PASS+1));
  else echo "- $NAME: FAIL" | tee -a "$RESULTS"; fi

  # A trace's own checks may have started a docker-compose stack (docker
  # compose up -d) — nothing ever tore it down, only this directory gets
  # deleted below, not the containers. They keep running and holding their
  # host ports, so the NEXT trace's docker compose up then fails outright
  # ("port is already allocated") for a reason that has nothing to do with
  # its own code (caught live: 002's leftover containers broke 003's run).
  # Tear down in the same place compose checks are ever run
  # (HARNESS_EVAL_CHECKS_HOST=1) — a no-op if nothing came up.
  if [ -f "$WT/docker-compose.yml" ] || [ -f "$WT/compose.yml" ]; then
    run_check "$WT" "docker compose down -v" >/dev/null 2>&1 || true
  fi
  rm -rf "$WT"
done
echo "" | tee -a "$RESULTS" >/dev/null
echo "**$PASS/$TOTAL** — total cost: \$${TOTAL_COST}" | tee -a "$RESULTS"
echo "Pass rate: $PASS/$TOTAL → $RESULTS"
[ "$PASS" = "$TOTAL" ] && [ "$TOTAL" -gt 0 ]
