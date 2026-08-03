#!/bin/bash
# Sandboxed one-shot Claude run — the ONLY sanctioned way to execute an agent
# against project code. Usage:
#   harness/docker/claude-run.sh <project-dir> <prompt> [output-file]
#
# Why --dangerously-skip-permissions is safe HERE and only here:
# the boundary is the container, not the permission config —
# throwaway mount, non-root user, dropped capabilities, resource limits.
# NEVER use that flag on the host.
set -eu
DIR=$(cd "$1" && pwd); PROMPT="$2"; OUT="${3:-/dev/stdout}"
IMG="${HARNESS_IMAGE:-harness-runner:latest}"
KIT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
# This script's only caller is evals/run.sh (grep confirms it — the agent's
# real work happens in interactive sessions, never through here), and an
# eval trace is verification against an already-solved task, not new
# multi-step reasoning — the same "reviewer"-tier judgment call agents/*.md
# already makes for read-only/checklist work. Defaulting off whatever the
# account's own default model is (seen live: claude-opus-4-8[1m], ~5x a
# mid-tier model's price, on a trace whose entire trajectory was "confirm
# this already-implemented feature still passes its checks") — override
# with HARNESS_EVAL_MODEL for a specific run if you need the primary tier.
#
# Tried haiku as the default here — reverted. Measured on the same trace
# (002-walking-skeleton): sonnet took 6 turns/$0.16/32s; haiku took 33
# turns/$0.20/143s and hit the EVAL_MAX_TURNS thrashing gate (plus an
# unexplained Write call on a trace that should need zero changes). Worse
# on every axis at once — slower, pricier in aggregate despite the lower
# per-token rate, and it failed the trace for thrashing rather than a real
# regression, which is exactly the kind of untrustworthy-verdict risk a
# eval gate can't afford. sonnet is the default again.
MODEL="${HARNESS_EVAL_MODEL:-sonnet}"

command -v docker >/dev/null || { echo "FATAL: docker is required for project runs (policy)."; exit 1; }
docker image inspect "$IMG" >/dev/null 2>&1 || docker build -t "$IMG" "$KIT_DIR/docker"

docker run --rm \
  -e ANTHROPIC_API_KEY \
  -v "$DIR":/work -w /work \
  --memory=4g --cpus=2 --pids-limit=512 \
  --cap-drop=ALL --security-opt no-new-privileges \
  "$IMG" \
  bash -c '
    # --dangerously-skip-permissions bypasses the allow/deny list, but not
    # the separate workspace-trust dialog — an untrusted /work makes claude
    # silently ignore every permissions.allow entry (seen live in CI: every
    # trace failed the same way, trajectory tools=[none], claude never took
    # a single action). No interactive session exists here to accept that
    # dialog, so pre-accept it the way the CLIs own error message says to.
    mkdir -p "$HOME"
    printf "%s" "{\"projects\":{\"/work\":{\"hasTrustDialogAccepted\":true}}}" > "$HOME/.claude.json"
    exec timeout 300 claude -p "$1" --model "$2" --output-format stream-json --verbose --dangerously-skip-permissions
  ' bash "$PROMPT" "$MODEL" \
  > "$OUT" 2>&1
# stream-json emits one JSON object per line (system/assistant/user/result) instead
# of a single summary object — harness/evals/run.sh needs the per-turn tool_use trajectory,
# not just the final result. The last line is still the same "result" summary.
