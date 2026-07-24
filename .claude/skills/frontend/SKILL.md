---
name: frontend
description: >
  Frontend conventions. Use when writing or editing React/TS code,
  components, hooks, state, styles, or frontend tests (web/, *.tsx, *.ts).
---
# Frontend

<!-- EDIT_ME: React+TS example (+Three.js as in CraftPlan, the fictional example project) -->

## Structure
1. web/src/features/<domain>/ — component + hook + types together. No components/ dumping grounds.
   Exception: web/src/ui/ holds a small, closed set of named cross-domain presentational
   primitives (established in ui-consistency) — legitimate at 2nd+ reuse across features
   (YAGNI gate), not a components/ dumping ground, as long as it stays small/closed (no
   index.ts barrel, no unrelated utility functions). If it grows into a grab-bag, split it
   back into feature-colocated files.
2. State: server state — TanStack Query, local — useState/useReducer.
   Global store — only after a second real consumer exists (YAGNI).
3. API calls — only through the generated/typed client, no fetch inside components.

## Tests
4. Test BEHAVIOR via Testing Library (what the user sees), not implementation details.
   Exception: when an acceptance criterion is itself about which Tailwind utility
   class is applied (e.g. a required focus-ring), asserting on `element.className`
   is the correct test — this is checking the criterion, not an implementation detail.
5. Three.js scene: unit tests cover PURE functions (geometry, part position math) —
   extract them from components. The canvas/render itself is not unit-tested;
   scene regression — screenshot in e2e, if you have one.
6. A hook with logic = its own test via renderHook.

## Forbidden
7. No any; unknown + narrowing. Domain types are imported, not duplicated.
8. Do not add a UI library/dependency without human approval.
