#!/bin/bash
# PostToolUse: formats the edited file. Never blocks (always exit 0).
command -v jq >/dev/null || exit 0
FILE=$(jq -r '.tool_input.file_path // empty')
[ -z "$FILE" ] && exit 0
case "$FILE" in
  *.go)       gofmt -w "$FILE" 2>/dev/null; golangci-lint run --fix "$FILE" 2>/dev/null || true ;;
  *.rb)       bundle exec rubocop -A "$FILE" 2>/dev/null || true ;;
  *.ts|*.tsx|*.js|*.jsx) npx prettier --write "$FILE" 2>/dev/null || true ;;
esac
exit 0
