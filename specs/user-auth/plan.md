# Plan: User Auth

Source: `specs/user-auth/spec.md`

Conventions confirmed from the codebase before planning:
- Routing: stdlib `net/http` `ServeMux` with Go 1.22+ method patterns (`cmd/api/main.go`), thin handlers holding small consumer-defined interfaces (see `Pinger` in `internal/handler/health.go`).
- Tests: plain `testing` package, table-driven, no testify (despite the generic `testing` skill template mentioning it — this repo's actual convention, established in `health_test.go`/`connect_test.go`/`migrate_test.go`, is stdlib-only; following existing code over the generic template).
- Integration tests hit a real dependency gated by an env var and `t.Skip` if unset (`TEST_DSN` in `internal/db/migrate_test.go`). We extend this pattern with `TEST_REDIS_ADDR` for the new Redis-backed packages — no mocking the data layer, per the testing skill.
- Migrations: goose, embedded FS, `IF NOT EXISTS`/idempotent style (`internal/db/migrations/001_initial_schema.sql`, `internal/db/migrate.go`).
- No sqlc/protobuf/openapi anywhere in the repo — confirmed via `go.mod` and file search.

Concrete values not pinned by the spec, decided here (flag if wrong): session TTL = 30 days; OTP TTL = 5 min (per spec); OTP request limit = 5/hour/phone (per spec); OTP verify attempt limit = 5/code (per spec). Redis key layout: `otp:{phone}:code`, `otp:{phone}:attempts`, `otp:{phone}:reqcount`, `session:{sessionID}`.

## Steps

1. **DB migration — add `phone_number`, relax `email`**
   - Files: `internal/db/migrations/002_add_phone_auth.sql`
   - What: `+goose Up`: `ALTER TABLE users ADD COLUMN phone_number VARCHAR(20) NULL UNIQUE;` and `ALTER TABLE users MODIFY email VARCHAR(255) NULL;`. `+goose Down`: reverse both (drop the column; restore `NOT NULL` on email — down migration assumes no phone-only rows exist yet in the environment it's run against, standard goose down caveat).
   - Test first: extend `internal/db/migrate_test.go` (or add `internal/db/migrate_phone_test.go`) with a case that runs `Up()` against `TEST_DSN`, then inserts a row with `phone_number` set and `email` NULL, and a row with the old shape (`email` set, `phone_number` NULL), asserting both succeed and a duplicate `phone_number` insert fails on the unique constraint.
   - Verify: `go test ./internal/db/... ` with `TEST_DSN` set; `TestUp_Idempotent` must still pass unmodified (second `Up()` call is a no-op).
   - Commit: separate commit, migration only — no Go application code in this commit.

2. **`internal/user` — phone-keyed user repository**
   - Files: `internal/user/user.go`, `internal/user/user_test.go`
   - What: `GetByPhone(ctx, db *sql.DB, phone string) (User, error)` (wraps `sql.ErrNoRows` into a package-level `ErrNotFound`), `Create(ctx, db *sql.DB, phone string) (User, error)`. Raw SQL only, no ORM. `User` struct: `ID uint64`, `PhoneNumber string`, `Email sql.NullString`, `CreatedAt time.Time`.
   - Test first (red): table-driven tests against real MySQL via `TEST_DSN` (skip if unset, matching `migrate_test.go`): not-found returns `ErrNotFound`; create-then-get round-trips; duplicate-phone create returns a wrapped MySQL unique-constraint error; verifies `Up()` has been run so schema exists (call `db.Up` in `TestMain` or per-test setup, consistent with how existing DB tests bootstrap).
   - Implement until green.
   - Verify: `go test ./internal/user/...` with `TEST_DSN` set.

3. **`internal/otp` — OTP lifecycle against Redis**
   - Files: `internal/otp/otp.go`, `internal/otp/otp_test.go`
   - What: `Service{Client *redis.Client, CodeTTL, RequestWindow time.Duration, MaxRequests, MaxAttempts int}` (constructor injects values from step-0 constants above, not hardcoded, so tests can use short-but-nonzero windows). Methods: `Request(ctx, phone) error` (generates 6-digit code, stores salted hash + resets attempts counter with `CodeTTL`, deletes/replaces any prior live code, enforces `MaxRequests` per `RequestWindow` via an incrementing counter key, returns the plaintext code to the caller for the stub log line — logging itself happens in the handler, not this package), `Verify(ctx, phone, code) error` (returns typed sentinel errors: `ErrNotFound`/`ErrExpired`/`ErrMismatch`/`ErrTooManyAttempts`; deletes the code key on successful verify so it's single-use; increments attempts and deletes the code key early if `MaxAttempts` is hit), `ErrRateLimited` for request flooding.
   - Test first (red): real Redis via `TEST_REDIS_ADDR` (skip if unset, same pattern as `TEST_DSN`). No `time.Sleep`: to test TTL expiry, call `Request` then directly `redisClient.Del`/`Expire(key, -1)` on the underlying key to deterministically simulate elapsed TTL, rather than waiting wall-clock time. Cases: request+immediate verify with correct code succeeds; wrong code fails and increments attempts; 6th wrong attempt returns `ErrTooManyAttempts` and invalidates the code (subsequent correct-code verify then fails with `ErrNotFound`); simulated-expired code returns `ErrExpired`; second successful verify of the same code returns `ErrNotFound` (already consumed); 6th `Request` within the window returns `ErrRateLimited` and stores no new code; a second `Request` before the first code is used invalidates the first (only the newest verifies).
   - Implement until green.
   - Verify: `go test ./internal/otp/...` with `TEST_REDIS_ADDR` set.

4. **`internal/session` — Redis-backed sessions**
   - Files: `internal/session/session.go`, `internal/session/session_test.go`
   - What: `Store{Client *redis.Client, TTL time.Duration}`. `Create(ctx, userID uint64) (sessionID string, error)` (cryptographically random ID via `crypto/rand`, stores `session:{id} -> userID` with `TTL`), `Validate(ctx, sessionID string) (userID uint64, error)` (`ErrNotFound` if missing/expired), `Delete(ctx, sessionID string) error`.
   - Test first (red): real Redis via `TEST_REDIS_ADDR`. Cases: create-then-validate round-trips the same `userID`; validate on an unknown ID fails; delete-then-validate fails; simulated expiry (manual `Expire(key, -1)`, no sleep) fails validate.
   - Implement until green.
   - Verify: `go test ./internal/session/...` with `TEST_REDIS_ADDR` set.

5. **`internal/handler/auth.go` — thin HTTP layer**
   - Files: `internal/handler/auth.go`, `internal/handler/auth_test.go`
   - What: handler-local consumer interfaces (mirroring the `Pinger` pattern in `health.go`) so tests use stubs, not real Redis/MySQL:
     - `OTPRequester interface { Request(ctx, phone string) error }`
     - `OTPVerifier interface { Verify(ctx, phone, code string) error }`
     - `SessionCreator interface { Create(ctx, userID uint64) (string, error) }`
     - `SessionDeleter interface { Delete(ctx, sessionID string) error }`
     - `UserFinder interface { GetOrCreateByPhone(ctx, phone string) (uint64, error) }` — thin adapter over `internal/user` that the handler package owns; if `internal/user.GetByPhone` returns `ErrNotFound`, adapter calls `Create`.
     - `OTPRequestHandler`, `OTPVerifyHandler`, `LogoutHandler` (each thin: validate phone format via regex/`net/netip`-style E.164 check, call into the interfaces above, translate sentinel errors to status codes per spec's acceptance criteria table), `SessionMiddleware(next http.Handler, sessions SessionValidator) http.Handler` (reads cookie, calls `Validate`, on success stores `userID` in request context via a package-level context key, on failure writes 401 and does not call `next`).
   - Test first (red): stub implementations of each interface (matching `stubPinger` style), table-driven per handler covering every acceptance criterion from the spec (invalid phone → 400, valid request → 202, correct verify → 200 + `Set-Cookie` with `HttpOnly`/`Secure`/`SameSite=Strict`, wrong code → 401, rate-limited → 429, too-many-attempts → 401, logout with/without cookie, middleware pass-through vs 401).
   - Implement until green.
   - Verify: `go test ./internal/handler/...`.

6. **Wire into `cmd/api/main.go`**
   - Files: `cmd/api/main.go`, `cmd/api/main_test.go`
   - What: construct `user` repo (holds `*sql.DB`), `otp.Service`, `session.Store` (both hold the already-connected `*redis.Client`), the `UserFinder` adapter, and register `POST /auth/otp/request`, `POST /auth/otp/verify`, `POST /auth/logout` (wrapped in `SessionMiddleware`) on the existing mux in `buildServer`. `buildServer`'s signature grows to accept the new dependencies (or a small options struct — decide at implementation time based on how many params it grows to, per YAGNI: don't introduce a struct until the parameter list is actually unwieldy).
   - Test first (red): extend `main_test.go`'s `TestBuildServer_NoPanic`-style test to assert the new routes are registered (e.g., a request to each returns something other than the mux's `404` fallback, using stub dependencies satisfying the new interfaces).
   - Implement until green.
   - Verify: `go build ./cmd/...` and `go test ./...`.

7. **Full-suite gate (no new files)**
   - Run `go build ./cmd/...`, `go vet ./...`, `golangci-lint run`, and `go test ./...` with both `TEST_DSN` and `TEST_REDIS_ADDR` set, confirming every step above is still green together (not just in isolation).

## Order

Tests precede implementation in every step (red/green pairs called out above) — no production code is written before its failing test exists, per the TDD requirement. The DB migration (step 1) is its own commit with no application code alongside it, so it can be reviewed/rolled back independently of the Go changes that depend on it. Steps 2–4 (repository, OTP, session — the data-layer packages) must land before step 5 (handlers), since the handler tests stub these packages' interfaces rather than reimplementing their logic. Step 6 (wiring) depends on 2–5 existing. Step 7 is a final cross-cutting check, not new code.

## Codegen

Not applicable — no sqlc, protobuf, or OpenAPI generation exists in this repo (confirmed: no `sqlc.yaml`, no `.proto` files, no OpenAPI spec, `go.mod` has no codegen tool dependencies). Nothing to regenerate.

## Risks

- **Session TTL / rate-limit numbers weren't pinned in the spec.** This plan assumes a 30-day session TTL; if that's wrong, it's a one-line constant change in `internal/session`, not a redesign. Flagging before implementation starts.
- **Down-migration safety.** Reversing "make `email` NOT NULL again" in step 1's down migration will fail if any phone-only (null-email) rows exist by the time someone runs `goose down`. Plan B: document this as a known one-way-in-practice migration (acceptable per db-migrations skill rule 2 — backward-compatible additive change; the down path is a dev-environment convenience, not a production rollback story).
- **Fixed-window rate limiting (not sliding).** `otp:{phone}:reqcount` with a flat 1-hour TTL means a burst just after a window resets could allow slightly more than 5 requests/hour at the boundary. Acceptable for this spec's threat model (abuse deterrence, not hard guarantee); plan B if it proves insufficient is a sliding-window counter (sorted set of timestamps), deferred until there's evidence of real abuse.
- **OTP stub logging.** Logging the plaintext OTP to stdout (per spec, standing in for SMS) must not leak into any log aggregation treated as long-term/exported storage without the team knowing — flag this to whoever owns log retention before this ships past local dev.
- **`buildServer` signature growth.** Adding 3+ new dependencies to `buildServer` risks an unwieldy parameter list; step 6 explicitly defers the struct-vs-params call to implementation time rather than guessing now (YAGNI).

## Out of scope guard

Do not touch:
- `internal/db/connect.go`, `internal/db/migrate.go`, `internal/redisconn/connect.go` — connection/retry logic is done, unrelated to auth.
- `internal/handler/health.go` and its test — `/healthz` must keep working unauthenticated and unmodified.
- `cmd/worker/*` — the worker has no auth surface; this spec is API-only.
- `internal/db/migrations/001_initial_schema.sql` — never edit a shipped migration; changes are additive in a new file (step 1).
- Any `analysis_jobs`/`participants`/`participant_metrics`/`job_summaries` code or ownership checks — explicitly non-scope per `specs/user-auth/spec.md`.
- `docker-compose.yml`, `docs/decisions.md` — no infra or architecture-decision changes are needed for this feature.
