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
- Any deviation from an explicit spec constraint must be surfaced explicitly (commit message/PR description), never left as only an inline code comment

## YAGNI gate
- Interface/abstraction — at the SECOND implementation, not before
- No `utils/`, `common/`, `helpers/`
- Every file must be required by the CURRENT spec

## Commands
- tests: `go test ./...` (set `TEST_DSN` / `TEST_REDIS_ADDR` to real MySQL/Redis to run integration tests; unset = skipped) · lint: `golangci-lint run` · build: `go build ./cmd/...`
- migrations (CLI): `goose -dir internal/db/migrations mysql "$DB_DSN" up` — api runs them automatically at startup via embedded FS

## Project map
docs/surface-map.md — read before tasks touching >1 module.
