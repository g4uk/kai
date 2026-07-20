#!/bin/bash
# Stop: don't let the agent finish with red tests. Scoped to changed packages.
command -v jq >/dev/null || exit 0
INPUT=$(cat)
[ "$(echo "$INPUT" | jq -r '.stop_hook_active')" = "true" ] && exit 0

cd "${CLAUDE_PROJECT_DIR:-.}" || exit 0

# EDIT_ME for your stack. Default — Go, scoped to changed packages.
CHANGED=$(git diff --name-only HEAD 2>/dev/null | grep -E '\.go$' || true)
[ -z "$CHANGED" ] && exit 0

PKGS=$(echo "$CHANGED" | xargs -n1 dirname | sort -u | while read -r d; do [ -d "$d" ] && echo "./$d"; done | tr '\n' ' ')
[ -z "$PKGS" ] && exit 0
if ! go test $PKGS > /tmp/claude-test.log 2>&1; then
  echo '{"decision":"block","reason":"Tests are red in changed packages. See /tmp/claude-test.log and fix before finishing. Full suite: go test ./..."}'
fi
exit 0
