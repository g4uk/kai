---
description: Append a row to docs/metrics.md — auto-fills what's derivable, asks only for judgment calls
---
Invariant #4: metrics are deliberately manual, but only the parts that need a
human's judgment. Everything objectively knowable should be filled in without
asking.

1. **Tokens / $** — compute directly, do not ask me to run anything:
   ```
   TRANSCRIPT=$(find "$HOME/.claude/projects" -name "${CLAUDE_CODE_SESSION_ID}.jsonl" 2>/dev/null | head -1)
   ```
   If `$CLAUDE_CODE_SESSION_ID` is unset or no file is found, fall back to: if
   `/cost`/`/usage` output is already visible earlier in this conversation,
   read it from there; otherwise ask me to run `/cost` (or `/usage`) once and
   paste the result. Do NOT guess.

   Otherwise, read every `"type":"assistant"` line's `message.usage` from
   `$TRANSCRIPT` (jq/python — whichever is available) and sum
   `input_tokens`, `output_tokens`, `cache_creation_input_tokens`,
   `cache_read_input_tokens` across the whole session; note the `model`
   field too. Tokens = those four summed. For $, apply current Anthropic API
   per-model pricing (input / output / cache-write / cache-read rates differ
   — use what you know to be current; if unsure, say the $ figure is
   approximate rather than inventing precision) to that same breakdown.
   Label the row's $ as computed-from-tokens vs from `/cost` if it matters
   for the Note field.

2. **LOC diff** — derive automatically, do not ask:
   ```
   git diff --shortstat <base>..HEAD
   ```
   `<base>`: the commit before this feature's first commit (e.g. the commit
   `specs/<feature>/spec.md` was added, or the tip of `main` before this
   branch). If ambiguous, ask me once which range to diff.

3. **Task / Approach** — infer from context (current `specs/<name>/` directory,
   recent commit messages) rather than asking, unless genuinely unclear.

4. Ask me only for the fields that require judgment:
   - **First-pass?** — did `/harness:verify` pass on the first attempt, yes/no
   - **Human min** — roughly how many minutes I spent reviewing/steering
   - **Note** — one line: what stood out, in my own words

5. Append one row to `docs/metrics.md` (create the table header from
   `templates/metrics.md.template` first if the file doesn't exist yet).
   Show me the row before appending — do not silently rewrite past rows.
