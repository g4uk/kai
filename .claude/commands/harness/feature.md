---
description: Full feature pipeline via subagents with approval stops
---
Run feature "$ARGUMENTS" through the full pipeline. You are the orchestrator: you do not
write code yourself; you pass CONDENSED results between agents. Check the dispatch matrix
(docs/dispatch-matrix.md): if the task is trivial (≤10 lines) — tell me the pipeline
is unnecessary and do it directly.

1. researcher: what this feature touches, which modules/risks
2. /harness:spec $ARGUMENTS based on the report → SHOW ME, wait for approval      ← STOP
3. /harness:plan $ARGUMENTS → SHOW ME, wait for approval                          ← STOP
4. test-writer: failing tests per plan
5. implementer: step-by-step implementation until green
6. reviewer: verdict on the diff. REQUEST_CHANGES → back to implementer (max 2 rounds,
   then escalate to me)
   Before advancing past this step: cross-check EVERY deviation named in the
   implementer's final report against specs/$ARGUMENTS/spec.md — each one needs
   its own amendment, not just the first/most obvious one. A report with N
   self-reported deviations and 1 spec.md amendment is a bug in this step, not
   a resolved handoff.
7. /harness:verify $ARGUMENTS
8. Remind me to run /harness:retro $ARGUMENTS after the merge

After each stage — one status line: stage / result / approx tokens.
