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
- No ORM — raw SQL via `database/sql` or `sqlx`
- No global mutable state
- Every feature starts with specs/<name>/spec.md — no spec, no code

## YAGNI gate
- Interface/abstraction — at the SECOND implementation, not before
- No `utils/`, `common/`, `helpers/`
- Every file must be required by the CURRENT spec

## Commands
- tests: `go test ./...` · lint: `golangci-lint run` · build: `go build ./cmd/...` · migrations: `goose up`

## Project map
docs/surface-map.md — read before tasks touching >1 module.
