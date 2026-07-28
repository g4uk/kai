# Kumite Analyzer

A self-hosted tool that accepts YouTube URLs and produces per-participant metrics and match summaries for kumite (martial arts sparring) footage. Built for coaches and athletes who want data-driven insights from competition video. Runs on a single Hetzner server via Docker Compose.

## Stack (decisions — in docs/decisions.md)
Go · Docker Compose · MySQL · Redis · Hetzner VPS

## Structure
- `cmd/api/` — HTTP server entry point
- `cmd/worker/` — async job processor entry point
- `internal/` — handlers, services, repository layer (no business logic in handlers)
- Nothing is generated and hand-editing is allowed everywhere
- `specs/<feature>/` — spec.md + plan.md for every feature

## Hard rules
- Every handler must verify the authenticated user owns the requested resource (user_id check on every query)
- Return errors, never panic; wrap with context: `fmt.Errorf("context: %w", err)`
- Handlers are thin — delegate all business logic to the service layer
- Handlers depend on small, consumer-defined single-method interfaces (e.g. `Pinger`, `OTPRequester`) — never import a concrete service/repo type directly into a handler
- No ORM — raw SQL via `database/sql` or `sqlx`
- No global mutable state
- Every feature starts with specs/<name>/spec.md — no spec, no code
- spec.md/plan.md must be committed in their own commit immediately after writing, before any implementation begins — an uncommitted spec/plan file does not satisfy "no spec, no code" (recurred 3x: walking-skeleton, ui, auth-me)
- Tests ship in the same commit as their implementation — never committed separately
- Any deviation from an explicit spec constraint must be surfaced explicitly (commit message/PR description) and noted with a brief amendment in the spec.md itself, never left as only an inline code comment or only in commit history (recurred: ui-consistency's `web/src/components/` → `web/src/ui/` path change was surfaced in the plan/commits but the spec.md kept the stale path until a later retro fixed it)
- A plan's "Out of scope guard" must never exclude a file/component the spec's Scope section names explicitly — a plan/spec contradiction must be surfaced as a decision to the human, not resolved silently by omission (recurred: ui-consistency's plan excluded AuthContext.tsx's LogoutButton from retrofit despite the spec naming it, shipping an unstyled control that violated acceptance criterion 1 until a later manual pass caught it)
- An acceptance criterion phrased as a universal claim across the whole surface ("any interactive element", "every screen") needs one explicit repo-wide check (grep/lint rule/manual sweep) in the plan, not just the sum of per-component tests (recurred: ui-consistency's two `<Link>` elements missed the focus-ring criterion because every test was scoped to a single component)
- A spec's UI/visual acceptance criteria that automated tests cannot prove (focus states, responsive overflow, visual consistency) are UNVERIFIED, not READY, until a real browser pass confirms them — this blocks `/harness:verify` from returning READY, it is not an optional follow-up (recurred: ui-consistency's flexbox overflow bug shipped past a 93-test-green suite and a full harness:verify pass, caught only once someone opened a browser)
- A plan step concluding "no code change needed" for a file the spec's Scope names explicitly must be backed by an empirical check (an instrumented run, a debug trace) — not inferred from reading the code's structure alone, especially for framework reconciliation/remount/lifecycle behavior (recurred: session-revalidation's plan asserted `ProtectedRoute` remounts on sibling-route navigation based on reading `router.tsx`'s JSX; a running instance proved React Router reuses the component instead, forcing a plan amendment mid-implementation)

## YAGNI gate
- Interface/abstraction — at the SECOND implementation, not before
- No `utils/`, `common/`, `helpers/`
- Every file must be required by the CURRENT spec

## Commands
- tests: `go test ./...` (set `TEST_DSN` / `TEST_REDIS_ADDR` to real MySQL/Redis to run integration tests; unset = skipped) · lint: `golangci-lint run` · build: `go build ./cmd/...`
- migrations (CLI): `goose -dir internal/db/migrations mysql "$DB_DSN" up` — api runs them automatically at startup via embedded FS

## Project map
docs/surface-map.md — read before tasks touching >1 module.
