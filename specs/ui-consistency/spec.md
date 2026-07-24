# Spec: ui-consistency

## Problem

The four existing SPA screens (login, OTP entry, job list, new-analysis form, job results) were built incrementally and use Tailwind inconsistently: some containers have layout utilities (`flex`, `gap-4`, `p-4`, `text-xl font-semibold`) while every `<input>`, `<button>`, `<table>`, and `<dl>` renders with zero classes — plain browser defaults. There's no shared page header (only `JobListPage` shows a logout action; `NewJobPage`/`JobResultsPage` have none), no consistent typography scale, and no visible styling for disabled/error/focus states that already exist functionally in the code. This spec makes every screen look and behave like one coherent, well-designed product instead of a patchwork of styled and unstyled elements.

## Scope

- Shared, hand-written UI primitives in `web/src/components/` (no new dependency) covering: primary `Button`, single-line text `Field`/`Input`, `Alert` (error/`role="alert"` display), `Table` (header/cell/row styling), `PageHeader` (title + Log out action), `EmptyState`, and a consistent loading/in-progress indicator
- Retrofitting `LoginPage`, `OtpPage`, `JobListPage`, `NewJobPage`, and `JobResultsPage` (and `AuthContext`'s `LogoutButton`) to use these shared primitives instead of raw unstyled elements
- Adding the missing `PageHeader` (title + Log out) to `NewJobPage` and `JobResultsPage` so all three authenticated screens share the same shell, not just `JobListPage`
- A centered, constrained-width card layout for the login/OTP screens (currently a left-aligned, full-bleed block)
- Consistent Tailwind default-palette usage for semantic states: primary actions, error/`role="alert"` text, disabled controls, and focus rings — using Tailwind v4's stock color/spacing/typography scale (e.g. `slate-*`, `blue-600`, `red-600`), no custom tokens
- A consistent two-level heading scale (one page-title style, one section-title style) applied across all screens
- Basic responsive behavior so no screen overflows horizontally at mobile widths, using existing Tailwind flex/wrap utilities (no dedicated per-breakpoint redesign)

## Non-scope

- Dark mode / `prefers-color-scheme` support — light theme only, per explicit decision for this pass
- A custom Tailwind theme (`@theme` tokens, custom color palette, custom spacing/typography scale) — this spec uses Tailwind's default scale as-is
- Any new UI/component-library dependency (e.g. shadcn, Radix, MUI, Headless UI) — primitives are hand-written Tailwind-styled React components, consistent with the project's current zero-extra-dependency frontend
- New pages, screens, routes, or functional behavior — this is a pure presentation/consistency pass over the five existing screens/components; no new API calls or business logic
- Animation/motion design beyond simple built-in Tailwind `transition`/`hover:`/`focus:` utilities already idiomatic for buttons and inputs
- Dedicated per-breakpoint mobile layouts (e.g. a separate mobile nav or hamburger menu) — only fluid/no-overflow behavior is in scope
- Icon set or illustration integration — text/typography only
- Backend/API changes of any kind — no new Go code, migrations, or endpoint changes (consistent with `specs/ui/spec.md`'s non-scope)
- A new `docs/decisions.md` entry — no new framework or dependency is introduced; Tailwind and React are already covered by decision 006

## Acceptance criteria

1. When any interactive element (button, input, or link acting as navigation) on any of the five screens receives keyboard focus, then a visible Tailwind focus-ring utility (e.g. `focus-visible:ring-2`) is rendered — none rely on the browser's native default outline alone.
2. When a form is submitting (`LoginPage`'s `isSubmittingPhone`, `OtpPage`'s `isSubmitting`, `NewJobPage`'s `submitting`), then the disabled input and submit button render a shared disabled style (reduced opacity + `disabled:cursor-not-allowed`) instead of the current unstyled native-disabled look.
3. When a validation or server error renders via a `role="alert"` element on `LoginPage`, `OtpPage`, or `NewJobPage`, then it uses the same shared `Alert` styling (consistent red text/border) on all three screens.
4. When the job list table renders on `JobListPage`, then its header row, cell padding, and row separators use the shared `Table` styling (consistent `px-*`/`py-*` cell padding and row dividers) instead of an unstyled native `<table>`.
5. When any primary submit action renders ("Send code", "Verify", "Submit"), then it is rendered via the shared `Button` component, so color, padding, hover, and disabled states are pixel-identical across screens.
6. When any single-line text input renders (phone number, OTP code, YouTube URL), then it is rendered via the shared `Field`/`Input` component, so border, padding, focus ring, and placeholder styling are identical across screens.
7. When `JobListPage`, `NewJobPage`, or `JobResultsPage` renders, then each displays the same shared `PageHeader` (page title + Log out action) — today only `JobListPage` shows a logout control.
8. When `LoginPage` or `OtpPage` renders, then both render inside the same centered, constrained-width card layout (e.g. `max-w-sm mx-auto`), not the current left-aligned full-width block.
9. When a loading/in-progress state renders ("Loading…" on `JobListPage`, "Analysis in progress…" on `JobResultsPage`), then both use the same shared loading presentation (identical text style, spacing, and container).
10. When the zero-jobs empty state renders on `JobListPage`, then it uses the shared `EmptyState` component with the same heading/body typography scale used elsewhere in the app.
11. When any page-level `<h1>` renders across the five screens, then all use one shared heading class combination; when any section-level heading renders (e.g. a participant's `<h2>` on `JobResultsPage`), then all of those share a single different class combination — no screen introduces a third one-off heading style.
12. When `JobResultsPage` renders a participant's metrics `dl`, then each `dt`/`dd` pair renders with consistent spacing/alignment via the shared styling, not an unstyled native `dl`.
13. When any screen is rendered at a 375px-wide viewport, then no screen produces page-level horizontal overflow — the job table and metric lists reflow or scroll within their own container rather than the page.
14. When `npm run build` (`tsc --noEmit` + `vite build`) runs after this change, then it completes with zero new TypeScript errors introduced by the styling change.
15. When the existing Vitest suite runs after this change, then all previously-passing tests (`LoginPage.test.tsx`, `OtpPage.test.tsx`, `JobListPage.test.tsx`, `NewJobPage.test.tsx`, `JobResultsPage.test.tsx`) still pass, since element roles/text content are unchanged even though their rendered classes are.

## Edge cases

1. **Simultaneous disabled + stale error** — a form is re-submitting (disabled state active) while a `role="alert"` from a prior failed attempt is still visible; the disabled style and the error style must both remain legible and not visually collide (criteria 2 and 3).
2. **Long content overflow** — a very long YouTube URL in the job table, or a very long metric key/value on the results screen, wraps within its cell/row rather than forcing the table or page to overflow horizontally (criterion 13).
3. **Zero-participant `done` job** — a job with status `done` but an empty `participants` array still renders the shared `PageHeader` and summary consistently, with no broken/empty gap where participant sections would go.
4. **Near-ceiling job count** — a job list at the ~50-job practical ceiling (per `specs/ui/spec.md`'s performance constraint) keeps consistent row height/padding throughout without the shared `Table` styles degrading (e.g. no layout shift as rows are added).
5. **Focus ring against varied backgrounds** — the shared focus-ring style remains visibly distinct both on the primary `Button` (colored background) and on plain page background, i.e. it isn't tuned only for one context (criterion 1).
6. **Repeated OTP failure** — resubmitting an invalid OTP code multiple times shows only one `Alert` at a time (no stacking/duplication) and the disabled-during-submit style is visually distinguishable from the error style even though both are active in quick succession.

## Constraints

- Compatibility: no new runtime dependency is added; styling is achieved with Tailwind utility classes and hand-written React components only (`web/package.json`'s dependency set is unchanged apart from what's already installed).
- Compatibility: no custom Tailwind `@theme` configuration — `web/src/index.css` continues to be `@import "tailwindcss";` with no added token layer.
- Compatibility: supports the same browser matrix as `specs/ui/spec.md` (latest two stable Chrome/Firefox/Safari); no IE/legacy support.
- Architecture: presentation-only — no Go code, migrations, API contract, or `internal/` changes.
- Performance: no regression to the existing 2-second render budget (`specs/ui/spec.md`'s constraint) for job counts up to 50; shared components must not introduce additional network requests or heavy client-side computation.
- Testing: per CLAUDE.md, tests updated for changed component structure ship in the same commit as the implementation; no separate follow-up test commit.
