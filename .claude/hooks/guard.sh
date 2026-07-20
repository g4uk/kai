#!/bin/bash
# PreToolUse(Bash): second layer after the permissions deny-list. exit 2 = block with explanation.
# First layer — permissions in settings.json (deny-by-default). This hook adds readable
# explanations and patterns the permissions syntax can't express.
command -v jq >/dev/null || { echo "BLOCKED: jq not found — guard cannot inspect the command (fail-closed). Install jq." >&2; exit 2; }
CMD=$(jq -r '.tool_input.command // empty')
[ -z "$CMD" ] && exit 0

deny() { echo "BLOCKED: $1" >&2; exit 2; }

echo "$CMD" | grep -qE 'rm -rf (/|~|\$HOME)'                    && deny "rm -rf on root/home"
echo "$CMD" | grep -qE 'git push.*(--force|-f)\b'                && deny "force push is forbidden"
echo "$CMD" | grep -qE 'git push[^|]*\+[a-zA-Z]'                 && deny "push with + (force via refspec) is forbidden"
echo "$CMD" | grep -qE 'DROP (TABLE|DATABASE)'                   && deny "DROP only via a migration with human review"
echo "$CMD" | grep -qiE '(migrate|goose|rake db).*(prod|production|live)' && deny "prod migrations — humans only"
echo "$CMD" | grep -qE '(cat|less|more|grep|head|tail) [^|;&]*\.env(\.|$|[^a-z])' && deny "reading .env / .env.* is forbidden"
echo "$CMD" | grep -qE '(sh|bash) -c'                            && deny "sh -c wrappers are forbidden — run the command directly so gates can see it"
echo "$CMD" | grep -qE 'chmod \+x /tmp'                          && deny "executable scripts from /tmp are forbidden"

# EDIT_ME: add project-specific denials
# WARNING: wide permissions like Bash(cat:*) or Bash(grep:*) read ALL files without pattern-matching.
# This guard hook is your defense; keep it updated. If the permission is truly needed,
# narrow it to a directory: Bash(cat:/app/src/**) or add EDIT_ME patterns here.

exit 0
