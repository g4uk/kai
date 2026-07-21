---
description: Generate plan.md from a spec
---
Read specs/$ARGUMENTS/spec.md. Create specs/$ARGUMENTS/plan.md:

## Steps — numbered, each: what we do / which files / how we verify
## Order — tests before implementation (TDD). DB migration = separate step and separate commit.
## Codegen — if the stack has generation (sqlc, protobuf, openapi) — explicit regeneration step
## Risks — what can go wrong, plan B
## Out of scope guard — files/directories we do NOT touch

Every step must end with green tests. Do NOT start implementing.
