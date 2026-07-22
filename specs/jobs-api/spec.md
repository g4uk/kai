# Spec: Jobs API

## Problem

Users are authenticated (user-auth spec) and the `analysis_jobs`/`participants`/`participant_metrics`/`job_summaries` tables already exist (walking-skeleton), but there is no way to submit a YouTube URL for analysis or retrieve a job's status and results. This spec delivers the three read/write endpoints — create, list, get-by-id — that the `ui` spec's job list, submission form, and results screen depend on.

## Scope

- `POST /jobs` — accepts a `youtube_url`, validates its format, creates an `analysis_jobs` row scoped to the authenticated `user_id` with `status='pending'`, and rejects duplicate non-failed submissions of the same URL by the same user with `409`
- `GET /jobs` — lists the authenticated user's own jobs, newest-first, with no pagination
- `GET /jobs/:id` — returns a single job's fields plus its participants (each with their metrics as key/value pairs) and its summary (if one exists), scoped to the authenticated `user_id`
- All three endpoints run behind the existing `SessionMiddleware` (from the user-auth spec) and read `user_id` via `UserIDFromContext`
- YouTube URL format validation for `watch?v=`, `youtu.be/`, and `/shorts/` URL shapes

## Non-scope

- Enqueueing jobs onto Redis or any worker-side consumption of a job queue — decision 002's queueing/processing side is deferred to a future "video-analysis-worker" spec; this spec only writes the initial DB row
- The video-analysis logic itself: nothing in this spec moves a job from `pending` to `processing`/`done`/`failed`, or writes `participants`, `participant_metrics`, or `job_summaries` rows — those are populated by a future worker spec; this API only reads whatever rows already exist
- Editing, cancelling, retrying, or deleting an existing job
- Pagination, filtering, or custom sort order on `GET /jobs` (fixed newest-first, no limit)
- Any endpoint for creating or editing participants/metrics directly through the API
- Rate limiting on `POST /jobs` (unlike the OTP endpoints, no rate limit is specified here)
- Schema changes — this spec uses the tables exactly as defined in `001_initial_schema.sql`; no new migration is introduced

## Acceptance criteria

1. When `POST /jobs` is called with a valid session and a `youtube_url` matching an accepted format (`youtube.com/watch?v=ID`, `youtu.be/ID`, or `youtube.com/shorts/ID`, with no existing non-failed job for that user+URL), then a new `analysis_jobs` row is created with `status='pending'` and `user_id` set from the session, and the response is `201` with `id`, `youtube_url`, `status`, `created_at`, `updated_at`.
2. When `POST /jobs` is called with a `youtube_url` that isn't a recognized YouTube URL format, then the response is `400` and no row is created.
3. When `POST /jobs` is called with an empty or missing `youtube_url` field, then the response is `400` and no row is created.
4. When `POST /jobs` is called by a user who already has a job for that exact `youtube_url` with status `pending`, `processing`, or `done`, then the response is `409` and no new row is created.
5. When `POST /jobs` is called by a user whose only existing job for that `youtube_url` has status `failed`, then a new row is created and the response is `201` (the 409 duplicate rule does not block resubmission after a failure).
6. When `POST /jobs`, `GET /jobs`, or `GET /jobs/:id` is called without a valid session cookie, then the response is `401` and no data is returned or written, per the existing `SessionMiddleware` behavior.
7. When `GET /jobs` is called by an authenticated user who owns exactly N jobs, then the response is `200` with exactly N entries, ordered newest-first by `created_at`, and containing no jobs belonging to any other `user_id`.
8. When `GET /jobs` is called by an authenticated user who owns zero jobs, then the response is `200` with an empty `jobs` array.
9. When `GET /jobs/:id` is called for a job owned by the authenticated user, then the response is `200` containing the job's fields, a `participants` array where each entry lists its `metrics` as key/value pairs (`metric_key`/`metric_value` from `participant_metrics`), and a `summary` field populated from `job_summaries` if a row exists.
10. When `GET /jobs/:id` is called for a job that has no `participants` rows yet, then the response is `200` with `participants: []` (never `null` or an error).
11. When `GET /jobs/:id` is called for a job that has no `job_summaries` row yet, then the response is `200` with `summary` as `null` (or omitted), not an error.
12. When `GET /jobs/:id` is called for an `:id` that does not exist in `analysis_jobs` at all, then the response is `404`.
13. When `GET /jobs/:id` is called for an `:id` that exists but belongs to a different `user_id`, then the response is `404` — identical to the non-existent case, with no field or message distinguishing the two.
14. When `GET /jobs/:id` is called with a non-numeric or otherwise malformed `:id` path segment, then the response is `400`, not `500` or a panic.
15. When `go build ./cmd/...` is run, the `api` binary compiles with the three new endpoints wired with zero errors.

## Edge cases

1. **Foreign tenant job ID** — `GET /jobs/999` where job `999` belongs to another user returns `404`, indistinguishable from a job ID that was never created (criterion 13).
2. **Resubmission after failure** — a user whose prior job for a URL ended in `failed` can submit that same URL again and gets a fresh `pending` job (criterion 5).
3. **Duplicate submission while a job is still in-flight** — submitting the same URL again while an existing job for it is `pending` or `processing` is blocked with `409` (criterion 4).
4. **Freshly created job with no participants yet** — `GET /jobs/:id` immediately after creation returns `participants: []`, not `null`, not a 500 from a failed join (criterion 10).
5. **Job with participants but no summary yet** — a job with `participants`/`participant_metrics` rows but no `job_summaries` row returns those participants normally with `summary: null` (criterion 11).
6. **Malformed job ID in the URL path** — `GET /jobs/abc` or `GET /jobs/-1` returns `400`, never an unhandled parse panic (criterion 14).
7. **Valid URL with extra query parameters** — `https://www.youtube.com/watch?v=abc123&t=42s&list=PL...` is still accepted; the format check matches on the presence of a valid `v=` video ID, not on the query string being exactly one parameter.
8. **Trailing slash / case variance on youtu.be** — `https://youtu.be/abc123/` and `https://YouTu.be/abc123` are still recognized as valid (host match is case-insensitive; a single optional trailing slash is tolerated).
9. **Same URL submitted by two different users** — each user's `409` duplicate check only considers their own `user_id`; a second user submitting a URL the first user already has pending is not blocked by the first user's job.

## Constraints

- Security: every query in all three handlers filters by the authenticated `user_id` from `UserIDFromContext` — `GET /jobs/:id` must use a single `WHERE id = ? AND user_id = ?` (or equivalent), never fetch-then-check-in-application-code, per CLAUDE.md's hard rule.
- Security: no new authentication mechanism — all three endpoints run behind the existing `SessionMiddleware` from the user-auth spec.
- No ORM — raw SQL via `database/sql`/`sqlx`, consistent with the rest of the codebase.
- Handlers stay thin and depend on small, consumer-defined interfaces (e.g. `JobCreator`, `JobLister`, `JobGetter`), mirroring the `OTPRequester`/`SessionCreator` pattern already established in `internal/handler/auth.go` — no concrete service/repo type imported directly into a handler.
- Compatibility: no migration changes — uses `analysis_jobs`, `participants`, `participant_metrics`, `job_summaries` exactly as defined in `001_initial_schema.sql`.
- Performance: `GET /jobs` and `GET /jobs/:id` respond within 200 ms under normal (all-up) conditions, consistent with the existing `/healthz` and auth latency bar.
- `participant_metrics.metric_value` is `DOUBLE` — the API serializes it as a JSON number; no change to the metric schema is needed to support arbitrary `metric_key`/`metric_value` pairs.
