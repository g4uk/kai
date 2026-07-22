# Plan: Jobs API

Source: `specs/jobs-api/spec.md`

Conventions confirmed from the codebase before planning:
- Routing: stdlib `net/http` `ServeMux` with Go 1.22+ method+path patterns (`cmd/api/main.go`); `GET /jobs/{id}` will use `r.PathValue("id")`.
- Handlers stay thin and depend on small, consumer-defined single-method-ish interfaces holding no concrete repo type (`Pinger` in `health.go`; `OTPRequester`/`OTPVerifier`/`UserFinder`/etc. in `auth.go`). Notably, `UserFinder.GetOrCreateByPhone` returns a plain `uint64`, not `internal/user.User` — the handler package never imports `internal/user`'s struct type, only reuses its sentinel errors is avoided too (auth.go doesn't check `user.ErrNotFound` at the handler layer). We follow the same isolation for jobs: the handler package defines its own minimal `Job`/`Participant`/`Metric` structs for interface returns and JSON serialization, and `cmd/api/main.go`'s adapter converts `internal/job` types into them — mirroring the `userFinder` adapter pattern exactly.
- Tests: plain `testing`, table-driven, no testify. Integration tests hit real MySQL gated by `TEST_DSN`, skipped if unset (`internal/user/user_test.go`, `internal/db/migrate_test.go`). Handler tests use hand-written stubs implementing the local interfaces (`auth_test.go`).
- No sqlc/protobuf/openapi in this repo — nothing to regenerate.
- **No migration needed.** `analysis_jobs`, `participants`, `participant_metrics`, `job_summaries` already exist from `001_initial_schema.sql`; the spec's non-scope explicitly excludes schema changes.

Concrete values not pinned by the spec, decided here (flag if wrong):
- Accepted YouTube URL shapes (case-insensitive host, http/https): `(www.)?youtube.com/watch?v={id}` (with any additional query params, e.g. `&t=`/`&list=`), `youtu.be/{id}` (optional single trailing slash), `(www.)?youtube.com/shorts/{id}`. Video ID pattern: `[A-Za-z0-9_-]{6,}` (permissive length, since exact YouTube ID rules aren't authoritative here).
- "Non-failed" for the duplicate-check (criterion 4/5) = job status is `pending`, `processing`, or `done`; only `failed` is excluded from blocking resubmission.
- Duplicate check + insert run inside one SQL transaction with `SELECT ... FOR UPDATE` scoping the existing-row check, to shrink (not eliminate — see Risks) the check-then-insert race window, since adding a DB-level unique constraint would require a migration the spec puts out of scope.

## Steps

1. **`internal/job` — job repository**
   - Files: `internal/job/job.go`, `internal/job/job_test.go`
   - What: `Job{ID uint64; UserID uint64; YoutubeURL string; Status string; CreatedAt, UpdatedAt time.Time}`, `Participant{ID uint64; Label string; Metrics []Metric}`, `Metric{Key string; Value float64}`, `JobDetail{Job; Participants []Participant; Summary sql.NullString}`. Sentinel errors `ErrNotFound`, `ErrDuplicate`. Functions:
     - `Create(ctx, db *sql.DB, userID uint64, youtubeURL string) (Job, error)` — inside a transaction: `SELECT id FROM analysis_jobs WHERE user_id=? AND youtube_url=? AND status <> 'failed' FOR UPDATE`; if a row exists, rollback and return `ErrDuplicate`; else `INSERT ... status='pending'`, commit, return the created row.
     - `ListByUser(ctx, db *sql.DB, userID uint64) ([]Job, error)` — `SELECT ... WHERE user_id=? ORDER BY created_at DESC`; returns `[]Job{}` (never `nil`) when there are no rows.
     - `GetByID(ctx, db *sql.DB, id, userID uint64) (JobDetail, error)` — single query: `SELECT ... FROM analysis_jobs WHERE id=? AND user_id=?` (wrap `sql.ErrNoRows` into `ErrNotFound` — this is the one query that enforces the foreign-tenant 404, per CLAUDE.md's hard rule); then one join query `SELECT p.id, p.label, m.metric_key, m.metric_value FROM participants p LEFT JOIN participant_metrics m ON m.participant_id = p.id WHERE p.job_id = ? ORDER BY p.id` scanned into `[]Participant` (no N+1 — a single query, grouped in Go by participant id); then `SELECT summary FROM job_summaries WHERE job_id = ?`, tolerating `sql.ErrNoRows` as a zero-value `sql.NullString` rather than an error.
   - Test first (red): gated on `TEST_DSN`, calling `db.Up` per the existing pattern. Cases: `Create` succeeds and returns `status='pending'`; second `Create` for the same user+URL while the first is `pending` returns `ErrDuplicate` and no new row; after manually updating the first row's status to `'failed'` (direct SQL in the test, since no status-update endpoint exists), a second `Create` for the same user+URL succeeds; `ListByUser` returns only the calling user's jobs, newest-first, and `[]Job{}` (not `nil`) for a user with none; `GetByID` for an owned job returns its participants (with metrics) and summary; `GetByID` for a job with zero participants returns `Participants: []Participant{}`; `GetByID` for a job with participants but no `job_summaries` row returns `Summary: sql.NullString{Valid: false}`; `GetByID` for a nonexistent id returns `ErrNotFound`; `GetByID` for an id that exists but under a different `userID` also returns `ErrNotFound` (same error, no distinguishing behavior — the foreign-tenant case).
   - Implement until green.
   - Verify: `go test ./internal/job/...` with `TEST_DSN` set.

2. **`internal/handler/jobs.go` — thin HTTP layer**
   - Files: `internal/handler/jobs.go`, `internal/handler/jobs_test.go`
   - What: handler-local types (deliberately not `internal/job`'s types — see isolation note above): `Job{ID uint64; YoutubeURL string; Status string; CreatedAt, UpdatedAt time.Time}`, `Participant{ID uint64; Label string; Metrics []Metric}`, `Metric{Key string; Value float64}`, `JobDetail{Job; Participants []Participant; Summary *string}`. Interfaces:
     - `JobCreator interface { Create(ctx context.Context, userID uint64, youtubeURL string) (Job, error) }`
     - `JobLister interface { ListByUser(ctx context.Context, userID uint64) ([]Job, error) }`
     - `JobGetter interface { GetByID(ctx context.Context, id, userID uint64) (JobDetail, error) }`
     `CreateJobHandler{Jobs JobCreator}`: decode body, validate `youtube_url` against the accepted-formats regex (400 on empty/malformed), read `userID` via `UserIDFromContext` (populated by the existing `SessionMiddleware`), call `Jobs.Create`; map `errors.Is(err, job.ErrDuplicate)` → 409 (importing `internal/job` only for this sentinel-error comparison, same precedent as `auth.go` importing `internal/otp` for `otp.ErrMismatch` etc.); other errors → 500; success → 201 + JSON.
     `ListJobsHandler{Jobs JobLister}`: `userID` from context, call `ListByUser`, always serialize as `{"jobs": [...]}` with `[]Job{}` (never `null`) when empty.
     `GetJobHandler{Jobs JobGetter}`: parse `r.PathValue("id")` via `strconv.ParseUint` — on failure, 400 *without* calling `Jobs.GetByID`; `userID` from context; call `GetByID`; `errors.Is(err, job.ErrNotFound)` → 404 (covers both the nonexistent-id and foreign-tenant cases identically, since the repo layer already scopes by `user_id`); success → 200 + JSON with `participants: []` (not `null`) and `summary: null` when absent.
   - Test first (red): stubs mirroring `auth_test.go`'s style (`stubJobCreator`, `stubJobLister`, `stubJobGetter`, each with a `calledP *bool` where the test needs to assert non-invocation). Table-driven per handler covering every acceptance criterion: malformed/empty URL → 400 (store not called); valid URL + stub success → 201 with correct JSON fields; stub `job.ErrDuplicate` → 409; stub generic error → 500; list with N stubbed jobs → 200 with exactly N entries; list with zero jobs → 200 body containing `"jobs":[]` (assert no literal `null`); get with full detail (participants + metrics + summary) → 200 with correct nested JSON; get with empty participants → `"participants":[]`; get with nil summary → `"summary":null`; get with `job.ErrNotFound` → 404; get with a non-numeric `id` path value → 400 and `stubJobGetter` not called.
   - Implement until green.
   - Verify: `go test ./internal/handler/...`.

3. **Wire into `cmd/api/main.go`**
   - Files: `cmd/api/main.go`, `cmd/api/main_test.go`
   - What: a `jobStore{db *sql.DB}` adapter implementing `handler.JobCreator`/`JobLister`/`JobGetter`, each method calling the matching `internal/job` function and converting `job.Job`/`job.Participant`/`job.Metric`/`job.JobDetail` into the handler-local types (including `sql.NullString` → `*string` for `Summary`), passing `job.ErrDuplicate`/`job.ErrNotFound` straight through unwrapped so the handler's `errors.Is` checks keep working. Register on the mux inside `buildServer` (whose signature grows by one `jobStore`-shaped parameter, reused for all three interfaces — consistent with `otpService` already satisfying both `OTPRequester` and `OTPVerifier`):
     - `mux.Handle("POST /jobs", handler.SessionMiddleware(&handler.CreateJobHandler{Jobs: jobs}, sessionValidator))`
     - `mux.Handle("GET /jobs", handler.SessionMiddleware(&handler.ListJobsHandler{Jobs: jobs}, sessionValidator))`
     - `mux.Handle("GET /jobs/{id}", handler.SessionMiddleware(&handler.GetJobHandler{Jobs: jobs}, sessionValidator))`
   - Test first (red): extend `main_test.go`'s existing "routes registered" style test with stub deps satisfying the three new interfaces, asserting each of the three routes returns something other than the mux's `404` fallback, and that unauthenticated requests to all three are rejected with 401 (proving `SessionMiddleware` is actually wrapping them, not just present in the file).
   - Implement until green.
   - Verify: `go build ./cmd/...` and `go test ./...`.

4. **Full-suite gate (no new files)**
   - Run `go build ./cmd/...`, `go vet ./...`, `golangci-lint run`, and `go test ./...` with `TEST_DSN` set, confirming steps 1–3 are green together, not just in isolation.

## Order

No migration step precedes this plan — unlike `user-auth`, the schema is already in place, so step 1 starts directly on the repository layer. Step 1 (`internal/job`) must land before step 2 (handlers), since the handler tests stub interfaces shaped around step 1's data, not the other way around. Step 3 (wiring) depends on both 1 and 2 existing. Step 4 is a final cross-cutting check, not new code. Tests precede implementation within every step (red/green pairs called out above).

## Codegen

Not applicable — no sqlc, protobuf, or OpenAPI generation in this repo (no `sqlc.yaml`, `.proto` files, or OpenAPI spec; confirmed via `go.mod` and file search). Nothing to regenerate.

## Risks

- **Check-then-insert race on the duplicate rule.** The `FOR UPDATE` transaction in `Create` narrows but does not fully eliminate a race between two concurrent `POST /jobs` calls for the same user+URL, because MySQL's `FOR UPDATE` only locks *existing* matching rows — it can't lock the absence of a row. A true guarantee would need a unique index (e.g. on `(user_id, youtube_url)` filtered somehow by status), which requires a migration the spec explicitly puts out of scope. Plan B if duplicates are observed in practice: revisit as a follow-up migration spec: adding a partial/generated-column unique index.
- **Handler/repo type duplication is intentional.** `handler.Job` and `job.Job` look redundant side by side; this mirrors the existing `userFinder` pattern (returns `uint64`, not `user.User`) to keep `internal/handler` free of concrete repo types per CLAUDE.md's hard rule. Flagging so a future pass doesn't "simplify" by importing `internal/job` types into the handler interfaces and reintroduce the coupling.
- **N+1 risk in `GetByID`.** Naively querying metrics per participant in a loop would be an N+1; the plan requires a single `LEFT JOIN` query grouped in Go, called out explicitly in step 1 so the implementer doesn't default to the simpler-looking loop.
- **Malformed `:id` must never reach the DB layer.** `strconv.ParseUint` happens in the handler before `Jobs.GetByID` is called at all — passing a bad ID straight to a `uint64`-typed SQL param would either error unpredictably or (worse) silently coerce; parsing early guarantees a clean 400 in every case, verified by asserting the stub was never invoked.
- **YouTube URL regex is a judgment call, not pinned by the spec.** The exact accepted patterns (documented above) are a reasonable reading of the spec's edge cases (extra query params, trailing slash, case-insensitive host) but not verbatim from the spec text — flagging in case the real requirement is stricter or looser.

## Out of scope guard

Do not touch:
- `internal/otp/*`, `internal/session/*`, `internal/redisconn/*`, `internal/user/*` — no auth or queueing changes are needed for this spec.
- `internal/db/migrations/*.sql` — no new migration; the existing schema is used as-is, per the spec's non-scope.
- `cmd/worker/*` — no Redis enqueueing or job-processing logic; deferred to a future worker spec, per the spec's non-scope.
- `internal/handler/health.go`, `internal/handler/auth.go` — untouched; `/healthz` and the auth endpoints keep working unmodified.
- `docker-compose.yml`, `docs/decisions.md` — no infrastructure or architecture-decision changes are needed for this feature.
- `specs/ui/*` — the SPA implementation itself is a separate spec/plan; this plan only builds the API it depends on.
