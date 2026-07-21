---
description: Generate spec.md for a feature
---
Create specs/$ARGUMENTS/spec.md following templates/spec.md.template
(if the template is missing from the repo, use the structure below):

# Spec: <name>
## Problem — 2-3 sentences, why the user needs this
## Scope — what's included
## Non-scope — what is EXPLICITLY excluded (minimum 3 items)
## Acceptance criteria — numbered, every item VERIFIABLE
   (format: "When X, then Y". No "should work well".
    For quantity fields — explicitly state behavior with qty/multiplicity)
## Edge cases — minimum 5 (including foreign tenant → 404, if multi-tenant)
## Constraints — compatibility, performance, security

Before generating: read the relevant code and docs/surface-map.md (if present).
Ask me clarifying questions if the scope is ambiguous. Do NOT write code.
