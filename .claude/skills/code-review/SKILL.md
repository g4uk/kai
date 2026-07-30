---
name: code-review
description: >
  Code review criteria for this project. Use when reviewing code,
  examining a diff, preparing a PR, or before creating a PR (self-review).
---
# Code Review — checklist

<!-- EDIT_ME: keep what's relevant to your stack, add your own rules from early reviews -->

## Blockers
1. [multi-tenant] Query on a tenant table without a tenant filter (company_id/org_id in WHERE)
2. Hand-editing generated code (sqlc/protobuf/openapi output)
3. Secrets/credentials in code or tests
4. Migration without down / backward-incompatible migration in one step
5. New import/package that isn't already in go.mod/package.json/requirements.txt —
   verify it exists and is the real package (agents hallucinate plausible-sounding
   package names; a hallucinated name can later get squatted with malware)

## Quality
6. N+1: DB calls inside a loop
7. Error handling per project convention (wrapping with context, sentinel errors)
8. Handler/controller stays thin: parsing + service call + error mapping
9. A test that mirrors the implementation (verifies the call, not the behavior) — rewrite
10. Abstraction with a single implementation "for the future" — remove (YAGNI)
11. Files outside the plan's scope (check against specs/*/plan.md out-of-scope guard)

## Remark format
file:line — issue — why it's a problem — how to fix
