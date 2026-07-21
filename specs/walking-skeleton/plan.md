# Plan: Walking Skeleton

## Out of scope guard

Do NOT touch or create:
- `docker/` — harness tooling, leave as-is
- `evals/`, `docs/`, `.claude/`, `.github/` — no changes
- Any file under `specs/` beyond this directory
- `cmd/api/` or `cmd/worker/` job-processing logic (none exists yet; keep it that way)
- Any endpoint other than `GET /healthz`

---

## Steps

### Step 1 — Go module + lint config
**What:** Bootstrap the module and establish the lint baseline before any Go code exists.

**Files:**
- `go.mod` — `module github.com/g4uk/kai`, `go 1.22`
- `.golangci.yml` — minimal config: `errcheck`, `govet`, `staticcheck`, `gofmt`; excludes `_test.go` from `errcheck` for `t.Log`/`t.Error` calls

**Verify:**
```
go mod tidy           # exits 0
golangci-lint run     # exits 0 (nothing to lint yet)
```

---

### Step 2 — DB migration (separate commit)
**What:** Create the initial Goose migration. This is a standalone commit so migration history stays decoupled from Go source history.

**Files:**
- `internal/db/migrations/001_initial_schema.sql`

**Schema (five tables from decision 003):**
```sql
-- +goose Up
CREATE TABLE IF NOT EXISTS users ( ... );
CREATE TABLE IF NOT EXISTS analysis_jobs ( ... );
CREATE TABLE IF NOT EXISTS participants ( ... );
CREATE TABLE IF NOT EXISTS participant_metrics ( ... );
CREATE TABLE IF NOT EXISTS job_summaries ( ... );

-- +goose Down
DROP TABLE IF EXISTS job_summaries;
...
DROP TABLE IF EXISTS users;
```

Full column list:
- `users`: `id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY`, `email VARCHAR(255) NOT NULL UNIQUE`, `created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP`
- `analysis_jobs`: `id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY`, `user_id BIGINT UNSIGNED NOT NULL`, `youtube_url TEXT NOT NULL`, `status VARCHAR(32) NOT NULL DEFAULT 'pending'`, `created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP`, `updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP`, `FOREIGN KEY (user_id) REFERENCES users(id)`
- `participants`: `id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY`, `job_id BIGINT UNSIGNED NOT NULL`, `label VARCHAR(128) NOT NULL`, `FOREIGN KEY (job_id) REFERENCES analysis_jobs(id)`
- `participant_metrics`: `id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY`, `participant_id BIGINT UNSIGNED NOT NULL`, `metric_key VARCHAR(128) NOT NULL`, `metric_value DOUBLE NOT NULL`, `FOREIGN KEY (participant_id) REFERENCES participants(id)`
- `job_summaries`: `id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY`, `job_id BIGINT UNSIGNED NOT NULL UNIQUE`, `summary TEXT NOT NULL`, `created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP`, `FOREIGN KEY (job_id) REFERENCES analysis_jobs(id)`

**Verify:**
```
# local smoke-check without MySQL — just parse validity:
goose -dir internal/db/migrations validate
# Full verify deferred to Step 10 (docker compose integration)
```

**Commit message:** `feat: initial schema migration (users, analysis_jobs, participants, participant_metrics, job_summaries)`

---

### Step 3 — Health handler: test first (RED → GREEN)
**What:** Define the `Pinger` interface and the `HealthHandler`, test all four states before writing the implementation.

**3a — Write test (RED)**

File: `internal/handler/health_test.go`

Test cases (table-driven):
| scenario | mysql pings | redis pings | want status | want body fields |
|---|---|---|---|---|
| both ok | ok | ok | 200 | `status:ok, mysql:ok, redis:ok` |
| mysql down | error | ok | 503 | `mysql:error, redis:ok` |
| redis down | ok | error | 503 | `mysql:ok, redis:error` |
| both down | error | error | 503 | `mysql:error, redis:error` |

Use `httptest.NewRecorder` and stub `Pinger` implementations. No real DB/Redis.

Verify: `go test ./internal/handler/...` → **compilation error** (package doesn't exist).

**3b — Implement (GREEN)**

File: `internal/handler/health.go`
```go
type Pinger interface { Ping(ctx context.Context) error }

type HealthHandler struct { DB Pinger; Redis Pinger }
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request)
```

Response body: `{"status":"ok|error","mysql":"ok|error","redis":"ok|error","error":"..."}` — `error` field present only when at least one dependency is down; `status` is `"error"` in that case.

Verify: `go test ./internal/handler/...` → **all pass**.

---

### Step 4 — DB connect package: test first (RED → GREEN)
**What:** `internal/db.Connect()` with exponential back-off retry.

**4a — Write test (RED)**

File: `internal/db/connect_test.go`

Test cases:
- `TestConnect_InvalidDSN`: call `Connect` with `root:wrong@tcp(127.0.0.1:1)/db` (nothing listening on :1); assert it returns a non-nil error within 5 s and does not loop forever.
- `TestConnectRetryConfig`: assert that the default retry config has `maxWait ≤ 30s` and `capInterval ≤ 5s` (read exported constants).

Verify: `go test ./internal/db/...` → **compilation error**.

**4b — Implement (GREEN)**

File: `internal/db/connect.go`
```go
// Connect opens a *sql.DB and pings until success or maxWait expires.
// Retry: 100ms initial, ×2 per attempt, cap 5s, total budget 30s.
func Connect(ctx context.Context, dsn string) (*sql.DB, error)
```

`*sql.DB` implements `Pinger` via a thin adapter (`DBPinger` struct with `Ping(ctx)`).

Verify: `go test ./internal/db/...` → **all pass** (no MySQL required; test uses invalid address).

---

### Step 5 — DB migrate: embed + Up function
**What:** Embed the SQL files from Step 2 and expose `Up(db)` for use at api startup. No new tests beyond the idempotency check.

**Files:**
- `internal/db/migrate.go`

```go
//go:embed migrations/*.sql
var migrationFS embed.FS

// Up runs all pending goose migrations against db.
func Up(db *sql.DB) error
```

Uses `goose.SetBaseFS(migrationFS)` + `goose.SetLogger(goose.NopLogger())` + `goose.Up(db, "migrations")`.

**Test (idempotency):**
- `internal/db/migrate_test.go` — integration test gated on `TEST_DSN` env var; skipped when env var is absent so unit test runs stay fast.
- When `TEST_DSN` is set: calls `Up(db)` twice, asserts second call returns nil.

Verify: `go test ./internal/db/...` (unit subset, no env var) → **all pass**.

---

### Step 6 — Redis connect package: test first (RED → GREEN)
**What:** `internal/redisconn.Connect()` with same back-off contract as DB.

**6a — Write test (RED)**

File: `internal/redisconn/connect_test.go`

Test cases:
- `TestConnect_InvalidAddr`: connect to `127.0.0.1:1`; assert non-nil error within 5 s.
- `TestConnectRetryConfig`: assert exported retry constants match spec.

Verify: `go test ./internal/redisconn/...` → **compilation error**.

**6b — Implement (GREEN)**

File: `internal/redisconn/connect.go`
```go
// Connect returns a *redis.Client that has successfully pinged, or an error.
func Connect(ctx context.Context, addr string) (*redis.Client, error)
```

`*redis.Client` wrapped in `RedisPinger` adapter so it satisfies `handler.Pinger`.

Verify: `go test ./internal/redisconn/...` → **all pass**.

---

### Step 7 — `cmd/api`: smoke test first (RED → GREEN)
**What:** Wire everything into the api binary with a `buildServer` helper testable without real dependencies.

**7a — Write test (RED)**

File: `cmd/api/main_test.go`
```go
// TestBuildServer_NoPanic asserts that buildServer() returns a non-nil
// *http.ServeMux without panicking when given stub Pingers.
func TestBuildServer_NoPanic(t *testing.T)
```

Verify: `go test ./cmd/api/...` → **compilation error**.

**7b — Implement (GREEN)**

File: `cmd/api/main.go`

Startup sequence:
1. Read config from env: `DB_DSN`, `REDIS_ADDR`, `PORT` (default `8080`)
2. Call `db.Connect(ctx, dsn)` — exits non-zero after retry exhaustion
3. Call `db.Up(sqlDB)` to run migrations
4. Call `redisconn.Connect(ctx, addr)` — exits non-zero after retry exhaustion
5. Build `*http.ServeMux` via `buildServer(dbPinger, redisPinger)` — registers `GET /healthz` and a catch-all 404
6. Start `http.ListenAndServe`

`buildServer` is an exported-for-test function (or unexported with test in same package).

Verify:
```
go build ./cmd/api/    # exits 0
go test ./cmd/api/...  # all pass
```

---

### Step 8 — `cmd/worker`: smoke test first (RED → GREEN)
**What:** Worker binary that connects to dependencies, then idles.

**8a — Write test (RED)**

File: `cmd/worker/main_test.go`
```go
// TestRunLoop_Cancels asserts that runLoop exits cleanly when ctx is cancelled.
func TestRunLoop_Cancels(t *testing.T)
```

`runLoop(ctx context.Context)` — the idle loop function extracted from `main`.

Verify: `go test ./cmd/worker/...` → **compilation error**.

**8b — Implement (GREEN)**

File: `cmd/worker/main.go`

Startup sequence:
1. Read `DB_DSN`, `REDIS_ADDR` from env
2. `db.Connect` + `redisconn.Connect` with same retry as api
3. `runLoop(ctx)` — `select { case <-ctx.Done(): return }` on a 5-second ticker; logs "worker idle" each tick
4. Graceful shutdown on `SIGTERM`/`SIGINT`

Verify:
```
go build ./cmd/worker/   # exits 0
go test ./cmd/worker/... # all pass
go test ./...            # full suite green
golangci-lint run        # no errors
```

---

### Step 9 — Dockerfile
**What:** Multi-stage build producing two lean runtime images.

**File:** `Dockerfile`

```dockerfile
# Stage: builder
FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/api    ./cmd/api
RUN CGO_ENABLED=0 go build -o /out/worker ./cmd/worker

# Stage: api
FROM alpine:3.19 AS api
COPY --from=builder /out/api /api
ENTRYPOINT ["/api"]

# Stage: worker
FROM alpine:3.19 AS worker
COPY --from=builder /out/worker /worker
ENTRYPOINT ["/worker"]
```

No migration files needed in the image — migrations are embedded in the `api` binary.

**Verify:**
```
docker build --target api    -t kumite-api:local .   # exits 0
docker build --target worker -t kumite-worker:local . # exits 0
```

---

### Step 10 — docker-compose.yml + full integration verify
**What:** Wire all four services with healthchecks and dependency ordering. Run every acceptance criterion.

**File:** `docker-compose.yml`

```yaml
services:
  mysql:
    image: mysql:8.0
    environment: { MYSQL_ROOT_PASSWORD: secret, MYSQL_DATABASE: kumite }
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost", "-psecret"]
      interval: 5s
      timeout: 5s
      retries: 10

  redis:
    image: redis:7-alpine
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 10

  api:
    build: { context: ., target: api }
    environment:
      DB_DSN: "root:secret@tcp(mysql:3306)/kumite?parseTime=true"
      REDIS_ADDR: "redis:6379"
      PORT: "8080"
    ports: ["8080:8080"]
    depends_on:
      mysql: { condition: service_healthy }
      redis: { condition: service_healthy }
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:8080/healthz"]
      interval: 5s
      timeout: 3s
      retries: 12

  worker:
    build: { context: ., target: worker }
    environment:
      DB_DSN: "root:secret@tcp(mysql:3306)/kumite?parseTime=true"
      REDIS_ADDR: "redis:6379"
    depends_on:
      mysql: { condition: service_healthy }
      redis: { condition: service_healthy }
```

**Verify (run each acceptance criterion in order):**
```bash
# AC1, AC2 (already verified in steps 7–8)
go build ./cmd/...
go test ./...

# AC3, AC4
docker compose up --build -d
sleep 30
curl -sf http://localhost:8080/healthz   # → {"status":"ok","mysql":"ok","redis":"ok"}

# AC5
docker compose stop mysql
curl http://localhost:8080/healthz       # → 503, mysql:error, redis:ok
docker compose start mysql

# AC6
docker compose stop redis
curl http://localhost:8080/healthz       # → 503, redis:error, mysql:ok
docker compose start redis

# AC7
docker compose stop mysql redis
curl http://localhost:8080/healthz       # → 503, both:error
docker compose start mysql redis

# AC8
docker compose down -v
docker compose up --build -d
docker compose exec mysql mysql -usecret -psecret kumite -e "SHOW TABLES;" | grep job_summaries

# AC10
curl -o /dev/null -w "%{http_code}" http://localhost:8080/   # → 404

docker compose down
```

---

## Codegen

No code generation in this stack (no sqlc, protobuf, or openapi). No regeneration step required.

---

## Risks

| Risk | Likelihood | Plan B |
|---|---|---|
| `go-sql-driver/mysql` ping returns `ErrBadConn` before server is ready, confusing the retry loop | Medium | Check for `driver.ErrBadConn` and transient dial errors explicitly; always retry on any non-nil ping error |
| Goose embedded FS path mismatch (`"migrations"` vs `"internal/db/migrations"`) | High first-try | Test `migrate.Up` against a real DB in the integration test (Step 5) before wiring into the binary |
| `wget` not available in the `alpine:3.19` image for the healthcheck | Low | Swap to `curl` (add `RUN apk add --no-cache curl` to the runtime stage) or use `CMD-SHELL` with a Go net dial check |
| mysql:8.0 `mysqladmin ping` outputs to stderr, breaking healthcheck detection | Low | Use `mysqladmin ping --silent` or switch to `mysql -e "SELECT 1"` |
| Port 8080 already in use on dev machine | Low | Document: change `ports: ["8080:8080"]` to `["18080:8080"]` locally |
| Migration `IF NOT EXISTS` on columns after initial create (future migrations) | n/a for step 2 | Only relevant for future migrations; note in migration comments |

---

## Order summary

```
Step 1  — go.mod + .golangci.yml                          [commit]
Step 2  — db/migrations/001_initial_schema.sql            [commit — isolated]
Step 3  — health handler (test → impl)                    [commit]
Step 4  — internal/db connect (test → impl)               [commit]
Step 5  — internal/db migrate (embed + Up)                [commit]
Step 6  — internal/redisconn connect (test → impl)        [commit]
Step 7  — cmd/api (test → impl)                           [commit]
Step 8  — cmd/worker (test → impl)  ← go test ./... green [commit]
Step 9  — Dockerfile                                      [commit]
Step 10 — docker-compose.yml + integration verify         [commit]
```
