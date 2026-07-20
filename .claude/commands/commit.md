---
description: Conventional commit from staged changes
---
Look at `git diff --staged`. Generate a conventional commit:
type(scope): description ≤72 chars, body — WHAT and WHY (not HOW).
Types: feat/fix/refactor/test/chore/docs. Scope = package/module.
If nothing is staged — say so and do nothing. Do not stage files yourself.
