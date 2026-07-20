---
description: Retro after a feature merge — feeds the harness
---
Feature $ARGUMENTS is merged. Do the following:
1. Compare specs/$ARGUMENTS/spec.md with the final implementation — list divergences
2. For each: is it a gap in the spec template, the plan, or CLAUDE.md? Propose a fix
3. Create evals/traces/NNN-$ARGUMENTS.md: prompt = the feature in one paragraph,
   checks = 3-4 verifiable criteria from the acceptance criteria
4. If a new convention emerged in the implementation — propose a CLAUDE.md line
Show everything to me for approval, do not commit anything yourself.
