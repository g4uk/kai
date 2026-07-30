#!/bin/bash
# Build script: bakes docs/metrics.md into a self-contained performance dashboard.
# No server, no build tooling — open dist/dashboard.html directly in a browser.
# Usage: ./build/dashboard.sh [path-to-project]   (defaults to .)
set -e
REPO="$(cd "$(dirname "$0")/.." && pwd)"
TARGET="$(cd "${1:-.}" && pwd)"

SRC="$REPO/harness/dashboard/template.html"
METRICS="$TARGET/docs/metrics.md"
DIST="$REPO/dist/dashboard.html"

[ -f "$SRC" ] || { echo "ERROR: $SRC not found"; exit 1; }

mkdir -p "$REPO/dist"

# Parse docs/metrics.md's pipe-table into a JSON array.
# Row shape: | Date | Task | Approach | Tokens | $ | LOC diff | First-pass? | Human min | Note |
parse_metrics() {
  local file="$1"
  [ -f "$file" ] || return 0
  awk -F'|' '
    function trim(s) { gsub(/^[ \t]+|[ \t]+$/, "", s); return s }
    # char-by-char, not gsub() — gsub replacement text has its own
    # backslash/& handling that fights with the string literal escaping
    # needed for a backslash-then-quote JSON escape
    function jsonesc(s,    out, i, c) {
      out = ""
      for (i = 1; i <= length(s); i++) {
        c = substr(s, i, 1)
        if (c == "\\") out = out "\\\\"
        else if (c == "\"") out = out "\\\""
        else out = out c
      }
      return out
    }
    # extract the first clean number token (e.g. "11.65" out of
    # "~$11.65 (est., intro pricing)") — a blanket [^0-9.] strip would
    # also keep the unrelated "." in "est." and glue it onto the number
    function firstnum(s) {
      if (match(s, /[0-9]+\.[0-9]+|[0-9]+/)) return substr(s, RSTART, RLENGTH)
      return "0"
    }
    NF < 10 { next }
    {
      date = trim($2); task = trim($3); approach = trim($4)
      cost = trim($6); fp = trim($8); mins = trim($9); note = trim($10)
    }
    date == "" || date == "Date" || date ~ /^-+$/ { next }
    {
      costnum = firstnum(cost)
      approx = (cost ~ /~/ || cost ~ /est/) ? "true" : "false"
      fpbool = (fp == "Yes") ? "true" : "false"
      minsnum = firstnum(mins)
      printf "{\"date\":\"%s\",\"task\":\"%s\",\"approach\":\"%s\",\"cost\":%s,\"approxCost\":%s,\"firstPass\":%s,\"mins\":%s,\"note\":\"%s\"},\n", \
        jsonesc(date), jsonesc(task), jsonesc(approach), costnum, approx, fpbool, minsnum, jsonesc(note)
    }
  ' "$file"
}

ROWS="$(parse_metrics "$METRICS")"
if [ -z "$ROWS" ]; then
  if [ -f "$METRICS" ]; then
    echo "WARN: no parseable rows in $METRICS — building with an empty dataset."
  else
    echo "NOTE: $METRICS not found yet — building with an empty dataset."
  fi
  JSON="[]"
else
  JSON="[$(printf '%s' "$ROWS" | tr -d '\n' | sed 's/,$//')]"
fi

# String-concat replacement (not awk sub()) so `&` in JSON content never
# gets special-cased. ENVIRON, not -v: awk's -v re-interprets backslash
# escapes in the assigned value (POSIX), which would silently corrupt the
# already-escaped JSON a second time (\" -> ", \\ -> \).
JSON_DATA="$JSON" awk '
  /__METRICS_JSON__/ { print "const METRICS_DATA = " ENVIRON["JSON_DATA"] ";"; next }
  { print }
' "$SRC" > "$DIST"

echo ">> Built: $DIST"
echo "   Open it directly in a browser — no server needed."
