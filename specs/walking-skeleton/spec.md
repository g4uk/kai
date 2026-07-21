# Spec: Walking Skeleton

## Problem

There is no runnable system yet — no Go module, no binaries, no Docker Compose stack. Before any feature work can begin, the full infrastructure path (code → build → containerise → run → health-check) must be proven end-to-end. This skeleton gives the team a single `docker compose up --build` command that starts all four services and confirms they are wired together correctly.

## Scope

- Go module at the repository root (`go.mod`, `go.sum`)
- `cmd/api` binary: HTTP server on `:8080`, connects to MySQL and Redis on startup, serves `GET /healthz`
- `cmd/worker` binary: connects to MySQL and Redis on startup, runs an idle loop (no jobs processed yet)
- `docker-compose.yml` with four services: `api`, `worker`, `mysql`, `redis` — all on a shared Compose network
- Initial Goose migration creating the five tables from decision 003: `users`, `analysis_jobs`, `participants`, `participant_metrics`, `job_summaries`
- `GET /healthz` response: JSON `{"status":"ok","mysql":"ok","redis":"ok"}` (or `"error"` values with HTTP 503 when a dependency is down)
- Startup retry/back-off in both binaries so containers can come up in any order

## Non-scope

- Authentication or authorization of any kind
- YouTube URL submission or any job-submission endpoint
- Redis job queueing, worker job processing, or any business logic
- HTTPS / TLS termination
- Production deployment, secrets management, or Hetzner provisioning
- Metrics, tracing, or structured logging beyond basic `log/slog` to stdout
- Any endpoint beyond `/healthz`

## Acceptance criteria

1. When `go build ./cmd/...` is run against a clean checkout, both `api` and `worker` binaries compile with zero errors.
2. When `go test ./...` is run, all tests pass (minimum: one smoke test per binary confirming the binary entry-point wires up without panicking).
3. When `docker compose up --build` is run, all four services (`api`, `worker`, `mysql`, `redis`) reach a running/healthy state within 60 seconds.
4. When `curl http://localhost:8080/healthz` is called after the stack is up, the response is HTTP 200 with body `{"status":"ok","mysql":"ok","redis":"ok"}`.
5. When MySQL is stopped (`docker compose stop mysql`) and `/healthz` is called, the response is HTTP 503 with body containing `"mysql":"error"` and a non-empty `"error"` field; the `redis` field remains `"ok"`.
6. When Redis is stopped (`docker compose stop redis`) and `/healthz` is called, the response is HTTP 503 with body containing `"redis":"error"`; the `mysql` field remains `"ok"`.
7. When both MySQL and Redis are stopped and `/healthz` is called, the response is HTTP 503 with both dependency fields set to `"error"`.
8. When `docker compose down -v && docker compose up --build` is run, Goose migrations apply without error and all five tables exist in the database.
9. When the `worker` container starts before MySQL migrations have finished, the worker retries its DB connection (with capped back-off, max 30 s) and does not exit with a non-zero code due to a transient connection failure.
10. When `GET /` or any undefined path is requested, the server returns HTTP 404.

## Edge cases

1. **Dependency not yet up at api startup** — api must retry the MySQL DSN ping with exponential back-off (e.g. 100 ms × 2, cap 5 s, max 30 s total) before exiting non-zero; same for Redis.
2. **Dependency not yet up at worker startup** — same retry logic; worker must not spin at 100 % CPU while waiting.
3. **MySQL DSN misconfigured** (bad password/host) — both binaries must exit non-zero within the retry window and log a clear error; they must not loop forever.
4. **Redis DSN misconfigured** — same behaviour as above.
5. **`/healthz` called mid-startup before DB is connected** — if the binary starts the HTTP listener before the dependency check completes, `/healthz` must return 503 (not 200 and not a connection-refused error).
6. **Goose migration applied twice** (idempotency) — `goose up` run a second time must succeed without error (standard Goose behaviour; the migration must use `IF NOT EXISTS` or equivalent).
7. **Port 8080 already in use on the host** — `docker compose up` must fail fast with a clear port-conflict error from Docker, not a silent hang.

## Constraints

- Go version: 1.22 or later (declare in `go.mod`)
- No ORM — raw `database/sql` with `go-sql-driver/mysql` for MySQL; `github.com/redis/go-redis/v9` for Redis
- No global mutable state; dependencies injected via constructor functions
- `golangci-lint run` must pass with the project's default config (no lint errors)
- MySQL image: `mysql:8.0`; Redis image: `redis:7-alpine`
- Migrations tool: `goose` (already referenced in CLAUDE.md); migration files live in `db/migrations/`
- `/healthz` must respond in under 500 ms under normal (all-up) conditions
- `docker-compose.yml` must declare `healthcheck` stanzas for `mysql` and `redis` so dependent services wait for readiness signals, not just process start
