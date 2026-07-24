# 006: ui-consistency — Tailwind consistency pass over the existing SPA

## Prompt

Make the existing 5-screen React SPA (login, OTP entry, job list, new-analysis
form, job results) visually consistent under Tailwind: introduce a small set
of shared, hand-written UI primitives (`Button`, `Field`, `Alert`, `Table`,
`PageHeader`, `EmptyState`, `StatusMessage`) and retrofit every screen to use
them instead of raw unstyled `<input>`/`<button>`/`<table>`/`<dl>` elements.
Add the page-header/logout shell that `NewJobPage` and `JobResultsPage` were
missing, center the login/OTP card layout, apply one consistent two-level
heading scale app-wide, and ensure every interactive element (including
navigation links) shows a visible focus ring rather than relying on the
browser's native default outline — using Tailwind's default color/spacing
scale only, no custom theme, no new dependencies, no dark mode.

## Checks

- [ ] cmd: cd web && npm ci && npm test -- --run
- [ ] cmd: cd web && npm ci && npm run typecheck && npm run build
- [ ] cmd: cd web && npm ci && npm run build && grep -q "focus-visible" dist/assets/*.css && grep -q "min-w-0" dist/assets/*.css
- [ ] (manual) run `docker compose up --build` and walk all 5 screens (login, OTP, job list, new-analysis, results) in a real browser at http://localhost:8081, tabbing through every interactive element to confirm a visible focus ring, and resizing to a 375px-wide viewport to confirm no page produces horizontal scroll — a full jsdom/Vitest pass (93 tests) and an automated harness:verify pass both missed two real bugs here (an unstyled Log-out control, and a flex/`min-width:auto` overflow bug on long metric keys) that only showed up once someone actually loaded the app and clicked around
