---
description: Retro after a feature merge — feeds the harness
---
Feature $ARGUMENTS is merged. Do the following:
1. Compare specs/$ARGUMENTS/spec.md with the final implementation — list divergences
2. For each: is it a gap in the spec template, the plan, or CLAUDE.md? Propose a fix
3. Create harness/evals/traces/NNN-$ARGUMENTS.md: prompt = the feature in one paragraph,
   checks = 3-4 verifiable criteria from the acceptance criteria. Each `cmd:` line
   runs standalone in its own throwaway container (harness/docker/exec.sh) — no state
   (running services, prior commands) survives between checks. If verifying
   something needs multiple steps (start a stack, then hit it), chain them with
   `&&`/`;` in ONE `cmd:` line, never split across checks expecting one to set up
   for the next. A `cmd:` value is executed AS THE SHELL COMMAND, verbatim — never
   append a human-readable note inline (e.g. "go test ./... (set FOO=bar to...)");
   put that as a separate `- [ ] (manual) ...` line instead.
4. If a new convention emerged in the implementation — propose a CLAUDE.md line
Show everything to me for approval, do not commit anything yourself.
