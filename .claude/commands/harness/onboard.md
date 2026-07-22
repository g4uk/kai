---
description: Interactive Stage 0 wizard — generate decisions.md, CLAUDE.md, metrics.md
---
Greenfield Stage 0 setup. Ask questions one at a time; do NOT write code.

1. Ask (multi-select): which files to generate this run —
   `docs/decisions.md`, `CLAUDE.md`, `docs/metrics.md` (any subset, or all three).
   If none selected — say there's nothing to do and stop.

2. For each selected file that already exists on disk: check whether it's
   still "raw" — unresolved `{{...}}` placeholders anywhere in it, or the
   metrics table with zero data rows — before deciding whether to ask.
   Raw/pristine content has nothing real to lose, so regenerate it directly,
   no question. Only if the file contains real filled-in content, ask "file
   exists with content — overwrite? (yes/no)" before touching it. "no" skips
   just that file; continue with the rest.

3. **docs/decisions.md** (if selected and not skipped): ask open-ended
   questions one at a time — do NOT suggest specific technologies or offer
   multiple-choice tech options:
   - What is the stack/architecture? (language, framework, DB, infra — in
     the user's own words)
   - What does the data model look like?
   - How/where will this deploy?
   - Is multi-tenancy needed?
   Write answers as numbered ADR entries following `templates/decisions.md.template`
   if that file exists in this repo; otherwise use this structure directly
   (don't skip just because the template file isn't there):
   ```
   # Architecture Decisions

   # 001: <decision>
   Why: <reasoning>
   Rejected: <alternatives and why>
   Cost: <the trade-off accepted>
   ```
   One numbered entry per answered question.

4. **CLAUDE.md** (if selected and not skipped): source the project's
   decisions from step 3's answers if it just ran this session; otherwise
   read the existing `docs/decisions.md` from disk (if neither exists, ask
   the stack/data-model/deploy questions from step 3 first). Ask two more
   questions:
   - Hard rules? (e.g. tenant isolation, error-handling convention, layering)
   - Any forbidden patterns / YAGNI specifics for this project?
   Fill `templates/CLAUDE.md.template` if it exists in this repo; otherwise
   use this structure directly, replacing every placeholder with real content
   from the answers above — never leave a `{{...}}` placeholder unfilled:
   ```
   # <PROJECT>

   <2-3 sentences: what it is, for whom, core value>

   ## Stack (decisions — in docs/decisions.md)
   <language/version · framework · DB · infra>

   ## Structure
   - <entry points>
   - <where the logic lives>
   - <what is generated and NEVER hand-edited, if anything>
   - specs/<feature>/ — spec.md + plan.md for every feature

   ## Hard rules
   - <tenant isolation / auth rule, if any>
   - <error handling convention>
   - <thin handlers / layers, if applicable>
   - Every feature starts with specs/<name>/spec.md — no spec, no code

   ## YAGNI gate
   - Interface/abstraction — at the SECOND implementation, not before
   - No utils/, common/, helpers/
   - Every file must be required by the CURRENT spec

   ## Commands
   - tests: <...> · lint: <...> · codegen: <...> · migrations: <...>
   ```

5. **docs/metrics.md** (if selected and not skipped): no questions — this
   file has no placeholders to fill, it's just the log's table header. Use
   `templates/metrics.md.template` if present, otherwise write this directly:
   ```
   # Harness Metrics Log
   # Fill in after EVERY significant session (/cost at session end)

   | Date | Task | Approach | Tokens | $ | LOC diff | First-pass? | Human min | Note |
   |------|------|----------|--------|---|----------|-------------|-----------|------|
   ```

6. Show a one-line summary per file: generated / skipped (existing, kept).
   A missing `templates/*.template` is never a reason to skip — the inline
   structures above are the fallback. Do not commit anything yourself.
