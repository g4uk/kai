#!/bin/bash
# PostToolUse(Write|Edit): catches secrets in a just-written file.
# The write already happened, so exit 2 = order the agent to remove the secret immediately.
command -v jq >/dev/null || { echo "secrets-scan: jq not found — check skipped" >&2; exit 0; }
FILE=$(jq -r '.tool_input.file_path // empty')
[ -z "$FILE" ] || [ ! -f "$FILE" ] && exit 0

if command -v gitleaks >/dev/null 2>&1; then
  if ! gitleaks detect --no-git --source "$FILE" -q >/dev/null 2>&1; then
    echo "BLOCKED: gitleaks found a secret in $FILE. Remove it immediately; secrets go through env only." >&2
    exit 2
  fi
  exit 0
fi

# Fallback: grep patterns (less accurate than gitleaks — install gitleaks)
PATTERNS='sk-[A-Za-z0-9]{20,}|sk-ant-[A-Za-z0-9-]{20,}|AKIA[0-9A-Z]{16}|ghp_[A-Za-z0-9]{36}|gho_[A-Za-z0-9]{36}|-----BEGIN [A-Z ]*PRIVATE KEY|xox[baprs]-[A-Za-z0-9-]{10,}|postgres(ql)?://[^ ]+:[^ ]+@'
if grep -qE "$PATTERNS" "$FILE" 2>/dev/null; then
  echo "BLOCKED: looks like a secret in $FILE (API key / private key / credentials in URL). Remove it immediately; use env variables." >&2
  exit 2
fi
exit 0
