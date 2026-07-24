# Plan: ui-consistency

Source: `specs/ui-consistency/spec.md`

Conventions confirmed from the codebase before planning:
- `web/src/index.css` is `@import "tailwindcss";` only — no `@theme` block, no `tailwind.config.js` (Tailwind v4 CSS-first setup via `@tailwindcss/vite`, per `specs/ui/plan.md`'s step 1). This plan does not add one, per the spec's non-scope.
- Existing screens (`LoginPage`, `OtpPage`, `JobListPage`, `NewJobPage`, `JobResultsPage`) use raw `<input>`/`<button>`/`<table>`/`<dl>` with zero classes; only outer containers have `flex`/`gap`/`p-4`/`text-xl font-semibold`. Confirmed by reading all five files directly, not just the spec's description.
- **Directory-naming deviation, surfaced up front per CLAUDE.md's "any deviation from an explicit spec constraint must be surfaced explicitly" rule**: the spec's Scope section names `web/src/components/` as the shared-primitives location. `.claude/skills/frontend`'s rule 1 forbids a `components/`/`common/`/`helpers/` dumping ground and mandates feature-colocation. Resolved (human confirmed): the shared primitives live in **`web/src/ui/`** instead — a deliberately named, small, cross-domain design-system layer (justified by 2nd+ reuse across the `auth` and `jobs` features, per CLAUDE.md's YAGNI gate), not a generic dumping ground. Every step below uses `web/src/ui/`, not `web/src/components/`.
- All five existing page test files (`LoginPage.test.tsx`, `OtpPage.test.tsx`, `JobListPage.test.tsx`, `NewJobPage.test.tsx`, `JobResultsPage.test.tsx`) query exclusively by role/label/text (`getByRole`, `getByLabelText`, `getByText`), never by className. Confirmed by reading all five test files. This means retrofitting classNames onto existing elements should not break any of them, **as long as the underlying semantic element/role/label/text is preserved** — this is the guardrail every retrofit step below verifies against.
- No `TanStack Query`/data-fetching change is in scope; `useJobs`/`useJob`/`useOtpFlow` hooks are untouched — this plan only changes what JSX/classNames the *page* components render, never the hooks' logic or their existing tests.
- Purely visual acceptance criteria (colors, spacing, centered-card layout, heading scale, focus-ring contrast, 375px no-overflow) are **not verifiable in jsdom/Vitest** — per CLAUDE.md's "if you can't test the UI, say so explicitly rather than claiming success," these are called out as manual browser-verification items in Step 9, not asserted by any automated test.

## Steps

1. **`Button` primitive**
   - Files: `web/src/ui/Button.tsx`, `web/src/ui/Button.test.tsx`
   - What: a `<button>` wrapper forwarding `type`, `disabled`, `onClick`, `children`, and an optional `variant` (`"primary"` only, for now — no second variant exists yet, so no variant prop complexity beyond what's needed). Bakes in one consistent Tailwind class string covering color, padding, hover, `focus-visible:ring-2`, and `disabled:opacity-50 disabled:cursor-not-allowed`.
   - Test first (red): renders as a real `<button>` with the given `type`/text; forwards `disabled` (asserted via `toBeDisabled()`); forwards `onClick`; asserts the rendered element's `className` contains `focus-visible:ring` and `disabled:cursor-not-allowed` (the one class-based assertion in this plan, justified because criteria 1/2/5 are explicitly about which classes are applied consistently, not just behavior).
   - Verify: `cd web && npm test -- --run ui/Button`.

2. **`Field` primitive (labeled single-line text input)**
   - Files: `web/src/ui/Field.tsx`, `web/src/ui/Field.test.tsx`
   - What: wraps a `<label>` + `<input>` pair (native association via `htmlFor`/`id`, generated from a required `label` prop if no `id` given) so `getByLabelText` keeps working unchanged. Forwards `type`, `value`, `onChange`, `disabled`. Bakes in one consistent Tailwind class string for the input (border, padding, `focus-visible:ring-2`, `disabled:opacity-50`).
   - Test first (red): `getByLabelText(label)` finds the input; typing calls `onChange`; `disabled` forwards; className contains `focus-visible:ring`.
   - Verify: `cd web && npm test -- --run ui/Field`.

3. **`Alert` primitive**
   - Files: `web/src/ui/Alert.tsx`, `web/src/ui/Alert.test.tsx`
   - What: a `<p role="alert">` wrapper with a consistent red text/border class string. Renders nothing (not an empty element) when `children` is `null`/`undefined`, so call sites can keep their existing `{error !== null && <Alert>...}` pattern or pass `error` straight through.
   - Test first (red): renders text with `role="alert"` when given children; renders nothing when given `null`.
   - Verify: `cd web && npm test -- --run ui/Alert`.

4. **`Table` primitives**
   - Files: `web/src/ui/Table.tsx`, `web/src/ui/Table.test.tsx`
   - What: thin wrapper components (`Table`, `TableHead`, `TableBody`, `TableRow`, `TableHeaderCell`, `TableCell`) rendering the corresponding native `<table>`/`<thead>`/`<tbody>`/`<tr>`/`<th>`/`<td>` elements — same semantics/roles as today, only with consistent Tailwind classes added (cell padding, row divider borders) plus an outer `<div className="overflow-x-auto">` around `<table>` for edge case 2/criterion 13 (long content wraps or the table itself scrolls, the page never does).
   - Test first (red): renders with `role="table"`/`"row"`/`"columnheader"`/`"cell"` preserved (same query shape `JobListPage.test.tsx` already uses: `getAllByRole("row")`).
   - Verify: `cd web && npm test -- --run ui/Table`.

5. **`PageHeader` primitive**
   - Files: `web/src/ui/PageHeader.tsx`, `web/src/ui/PageHeader.test.tsx`
   - What: renders an `<h1>` (the shared page-title class combination, criterion 11) with the given title text, plus a right-aligned slot rendering `children` (the existing `LogoutButton` from `AuthContext`, unchanged) — this is purely a layout wrapper, it does not know about auth.
   - Test first (red): renders the given title as a heading (`getByRole("heading", { name })`); renders passed-in children (e.g. a stub logout button) alongside it.
   - Verify: `cd web && npm test -- --run ui/PageHeader`.

6. **`EmptyState` and `StatusMessage` primitives**
   - Files: `web/src/ui/EmptyState.tsx`, `web/src/ui/StatusMessage.tsx`, `web/src/ui/EmptyState.test.tsx`, `web/src/ui/StatusMessage.test.tsx`
   - What: `EmptyState` — a consistent heading+body+optional-link block (used today only by `JobListPage`'s zero-jobs state). `StatusMessage` — a single consistent container/text style for one-line status text (used for "Loading…" and "Analysis in progress…", criterion 9); it takes `children` as the message, no built-in text of its own (no premature generalization beyond what criterion 9 needs).
   - Test first (red): `EmptyState` renders heading text, body text, and (when given a `to`/`label`) a link with that accessible name. `StatusMessage` renders its children inside the shared container.
   - Verify: `cd web && npm test -- --run ui/EmptyState ui/StatusMessage`.

7. **Retrofit `LoginPage` + `OtpPage`** *(auth screens use every primitive except `Table`/`PageHeader`)*
   - Files: `web/src/features/auth/LoginPage.tsx`, `web/src/features/auth/OtpPage.tsx`, `web/src/features/auth/LoginPage.test.tsx`, `web/src/features/auth/OtpPage.test.tsx`
   - What: swap the raw `<input>`/`<button>`/`<p role="alert">` for `Field`/`Button`/`Alert`; wrap both screens' root in a shared centered card layout (`max-w-sm mx-auto`, criterion 8) — this is the one piece of new page-level JSX, not a new primitive, since only these two screens need it.
   - Test first: no new test *cases* are required (existing role/label/text queries already cover the behavior) — but re-run the existing suites first to establish the green baseline being preserved, then add one assertion to each confirming the card container's `max-w-sm` wrapper is present (`container.querySelector` on the root, since there's no accessible role for "is this a centered card") so criterion 8 has a regression guard.
   - Verify: `cd web && npm test -- --run auth`.

8. **Retrofit `JobListPage`, `NewJobPage`, `JobResultsPage`** *(adds the missing `PageHeader` to the latter two)*
   - Files: `web/src/features/jobs/JobListPage.tsx`, `web/src/features/jobs/NewJobPage.tsx`, `web/src/features/jobs/JobResultsPage.tsx`, and their three `.test.tsx` files
   - What:
     - `JobListPage`: replace the raw `<table>` with `Table`/`TableHead`/etc., the empty-state block with `EmptyState`, the `"Loading…"` text with `StatusMessage`, and wrap the existing `<h1>`+`LogoutButton` in `PageHeader`.
     - `NewJobPage`: replace the raw `<input>`/`<button>`/alerts with `Field`/`Button`/`Alert`; **add** `PageHeader` with a "Log out" action (today it has none — this is the fix for criterion 7).
     - `JobResultsPage`: replace `"Loading…"`/`"Analysis in progress…"` with `StatusMessage`; **add** `PageHeader`; apply the shared section-heading class to each participant's `<h2>`; style the `dl`/`dt`/`dd` metrics list consistently (criterion 12).
   - Test first: extend `NewJobPage.test.tsx` and `JobResultsPage.test.tsx` with one new case each asserting a "Log out" button (`getByRole("button", { name: /log out/i })`) is present — this is new, spec-mandated behavior (criterion 7), so it gets a real red-then-green test, unlike step 7's pure retrofit. Re-run all three existing suites first to confirm the green baseline before editing.
   - Verify: `cd web && npm test -- --run jobs`.

9. **Heading-scale + responsive pass + full-suite gate** *(no new files; cross-cutting cleanup + verification)*
   - Files: any of the five page components, touched only if step 7/8's retrofit left a one-off heading class combination behind (grep check, see Verify).
   - What: confirm exactly two heading class combinations exist repo-wide (one for every `<h1>` via `PageHeader`, one for every section `<h2>`) and that the `Table`/metrics-list containers have `overflow-x-auto`/wrapping applied per step 4/8.
   - Verify (automated): `grep -rn "className=\".*text-.*font-" web/src/features web/src/ui` and manually confirm no third one-off combination exists; `cd web && npm run typecheck && npm run build && npm test -- --run` (criteria 14/15, full green suite); `go build ./cmd/... && go test ./...` as a cheap regression check that nothing under `internal/`/`cmd/` was touched.
   - Verify (manual, per CLAUDE.md's "start the dev server and use the feature in a browser" rule — required here since criteria 1, 8, 9 (partially), 11, and 13 are not jsdom-testable): `cd web && npm run dev`, walk all five screens at a normal desktop width and at a 375px-wide viewport (browser devtools device toolbar), confirming: visible focus ring on every interactive element via Tab-key navigation (criterion 1, edge case 5), the login/OTP card is centered and constrained (criterion 8), no page produces horizontal scroll at 375px (criterion 13, edge case 2 with a long pasted URL), and the two-level heading scale reads consistently across all five screens (criterion 11).

## Order

Steps 1–6 (primitives) have no dependencies on each other and could be built in any order or in parallel, but are listed in the sequence they're first consumed (Button/Field/Alert by the auth screens in step 7 first) to keep the diff readable. Step 7 depends on steps 1–3 (`Button`, `Field`, `Alert`). Step 8 depends on steps 1–6 (uses all six primitives). Step 9 depends on 7 and 8 being complete — it is the cross-cutting gate, not new functionality. No DB migration exists in this spec (presentation-only, per the spec's Constraints), so the "migration = separate commit" rule from the template doesn't apply; **each step above is its own commit**, tests and implementation together per CLAUDE.md ("tests ship in the same commit as their implementation").

Every step's test-first requirement means: for steps 1–6, write the new component's test file first (it fails — the component doesn't exist), then implement until green. For steps 7–8, the *existing* suites are the red/green gate for the retrofit itself (any accidental role/label/text change turns them red), and the one or two genuinely new assertions (card wrapper in step 7, Log out button in step 8) get their own red-then-green cycle.

## Codegen

Not applicable — no sqlc/protobuf/OpenAPI in this repo (confirmed by `specs/ui/plan.md`'s prior finding, unchanged). `web/src/api/types.ts` is unaffected by this spec (no wire-shape changes).

## Risks

- **Adding a wrapper `<div>` around `<table>` (step 4) could shift `getAllByRole("row")` counts or hide the `"table"` role from existing queries.** Plan B: keep the wrapper div role-neutral (no `role` attribute of its own) and re-run `JobListPage.test.tsx` immediately after step 4/8 lands; if a query breaks, the fix is adjusting the wrapper, never the test (the test's role-based query is the source of truth for "did we preserve semantics").
- **`Field`'s auto-generated `id`/`htmlFor` could collide if two `Field`s with the same `label` render on one page simultaneously.** Not currently possible (each screen has at most one of a given label), but if it ever happens, `getByLabelText` would become ambiguous. Plan B: accept an optional explicit `id` prop from day one (cheap, already planned into step 2) so any future collision has an escape hatch without revisiting the primitive.
- **The "well-designed pages" bar (spec's Problem statement) is inherently more subjective than the rest of CLAUDE.md's "no should-work-well" acceptance-criteria rule allows.** Mitigated by grounding every criterion in a concrete, checkable class/behavior (specific Tailwind utility names, specific components used) rather than aesthetic judgment — but the manual-verification step (9) still involves a human eyeballing the result, which is called out explicitly rather than hidden behind a green test suite.
- **Retrofitting five pages in two batched steps (7, 8) risks a large diff that's harder to review than one page at a time.** Plan B if review pushback happens: split step 8 into three separate commits (one per page) — the plan's grouping is a default for velocity, not a hard requirement, and CLAUDE.md's "tests ship with implementation" rule is satisfied either way since each page's own test file would still travel with its own retrofit.
- **`web/src/ui/` naming choice (see the deviation note above) could still read as "components/ by another name" to a future reviewer applying the frontend skill literally.** Mitigated by keeping the directory small and closed (six named primitives, no `index.ts` barrel/grab-bag, no unrelated utility functions) — if it grows into a dumping ground later, that's a signal to split it back into feature-colocated files, not evidence this plan's choice was wrong today.

## Out of scope guard

Do not touch:
- `internal/*`, `cmd/*`, `internal/db/migrations/*.sql` — no Go/backend/schema changes; this spec is presentation-only.
- `web/src/api/client.ts`, `web/src/api/types.ts` — no wire-shape or fetch-logic changes.
- `web/src/features/auth/useOtpFlow.ts`, `web/src/features/jobs/useJobs.ts`, `web/src/features/jobs/useJob.ts` — data-fetching/state-machine hooks are unchanged; only the JSX/classNames in the page components that consume them change.
- `web/src/app/AuthContext.tsx`, `web/src/app/ProtectedRoute.tsx`, `web/src/app/router.tsx` — routing/auth logic untouched (the only touch is `LogoutButton`'s call sites moving inside `PageHeader`, not `LogoutButton`'s own implementation).
- `web/src/index.css`, no new `tailwind.config.js`/`@theme` block — per the spec's non-scope, Tailwind's default scale is used as-is.
- `docker-compose.yml`, `Dockerfile`, `web/nginx.conf` — no deployment/infra changes; this spec ships inside the existing `web` service's build.
- `docs/decisions.md` — no new entry required (no new framework/dependency introduced), per the spec's non-scope.
- `package.json` dependencies — no new runtime/dev dependency is added anywhere in this plan.
