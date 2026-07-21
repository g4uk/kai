---
description: Append a row to docs/metrics.md — auto-fills what's derivable, asks only for judgment calls
---
Invariant #4: metrics are deliberately manual, but only the parts that need a
human's judgment. Everything objectively knowable should be filled in without
asking.

1. **Tokens / $ / duration** — if `/cost` or `/usage` output is already visible
   earlier in this conversation, read it from there. Otherwise ask me to run
   `/cost` (or `/usage`, depending on CLI version) now and paste the result —
   do NOT guess these numbers.

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
   - **First-pass?** — did `/verify` pass on the first attempt, yes/no
   - **Human min** — roughly how many minutes I spent reviewing/steering
   - **Note** — one line: what stood out, in my own words

5. Append one row to `docs/metrics.md` (create the table header from
   `templates/metrics.md.template` first if the file doesn't exist yet).
   Show me the row before appending — do not silently rewrite past rows.
