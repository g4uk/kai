---
description: Verify phase — prove acceptance criteria before finishing a feature
---
Read specs/$ARGUMENTS/spec.md and specs/$ARGUMENTS/plan.md. Run verification:

1. ACCEPTANCE: walk the criteria ONE BY ONE. For each — evidence:
   a specific test (name + run) or a command with output. Format:
   AC-N → evidence → PASS/FAIL. No evidence = FAIL, not "looks fine".
2. CONSTRAINTS: walk the spec's Constraints section ONE BY ONE too, same
   evidence bar as acceptance criteria. Any numeric constraint (performance,
   rate limits, TTLs) with no measurement = FAIL, not "should be fine".
3. SCOPE: git diff main...HEAD — list files NOT required by any plan step,
   and files from the out-of-scope guard. Any found = FAIL.
4. YAGNI: new interfaces/abstractions with a single implementation = remark.
5. Full test suite and linter run.

Summary: VERDICT: READY | NOT READY + list of FAILs.
If NOT READY — do NOT fix silently: show the list, wait for my decision.
