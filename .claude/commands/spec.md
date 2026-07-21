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
    For quantity fields — explicitly state behavior with qty/multiplicity.
    If a criterion names an error code/label in parentheses, state explicitly
    whether it must appear verbatim in the response, e.g. a body field or
    header (machine-readable) or is descriptive only, e.g. a name for this table)
## Edge cases — minimum 5 (including foreign tenant → 404, if multi-tenant)
## Constraints — compatibility, performance, security
   (any numeric performance/security constraint here must either be promoted
    into a numbered acceptance criterion, or explicitly flagged as unverified
    in this pass — /verify checks Constraints too, not just Acceptance criteria)

Before generating: read the relevant code and docs/surface-map.md (if present).
Ask me clarifying questions if the scope is ambiguous. Do NOT write code.
