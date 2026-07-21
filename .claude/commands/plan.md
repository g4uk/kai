---
description: Generate plan.md from a spec
---
Read specs/$ARGUMENTS/spec.md. Create specs/$ARGUMENTS/plan.md:

## Steps — numbered, each: what we do / which files / how we verify
## Order — tests before implementation (TDD). DB migration = separate step and separate commit.
## Codegen — if the stack has generation (sqlc, protobuf, openapi) — explicit regeneration step
## Risks — what can go wrong, plan B
## Out of scope guard — files/directories we do NOT touch

Before finalizing, cross-check each step's literal implementation detail
against every acceptance criterion it touches for contradictions (e.g. a step
saying "wrap X in auth middleware" vs. a criterion requiring X to work
unauthenticated) — resolve here, don't leave it for the implementer to catch
mid-build.

Every step must end with green tests. Do NOT start implementing.
