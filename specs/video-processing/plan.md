# Plan: Video Processing

Implements `specs/video-processing/spec.md`. New package `internal/video` holds the download/probe/analyze pipeline; `internal/job` gains two repository functions to persist results; `cmd/worker/main.go`'s `processTick` is redesigned from "advance on a fixed tick delay" to "advance to processing, then run the real pipeline synchronously, then advance to done/failed" — collapsing today's two-phase-per-tick stub into one per-job call.

## Steps

1. **Red: `internal/video` orchestration tests** — `internal/video/video_test.go` (new file). Define fakes for `Downloader`, `Prober`, `Analyzer` interfaces and write table-driven tests against a `Pipeline` orchestrator type: success-first-try, transient-failure-then-succeed (asserting exactly 2 attempts, backoff durations passed to a fake clock/sleep func — never real `time.Sleep`, so tests stay fast), all-3-attempts-fail (asserting `failed` result + zero partial writes + backoff sequence 1x/2x the injected base), per-attempt timeout exceeded (fake `Analyzer` that blocks past a short injected timeout), and temp-dir cleanup happening in all four cases (fake filesystem or real `t.TempDir()`-based check). No real `yt-dlp`/`ffmpeg`/network involved.
   *Verify:* `go test ./internal/video/...` fails to compile (red — `video.go` doesn't exist yet).

2. **Green: `internal/video/video.go`** — implement `Downloader`, `Prober`, `Analyzer` interfaces, their result structs (`Metadata{Duration, Width, Height, FPS}`, `AnalysisResult{Participants []ParticipantResult}`, `ParticipantResult{Label string, ActivityScore float64}`), and `Pipeline` with `Run(ctx, youtubeURL string) (Result, error)` — retry loop using `github.com/sethvargo/go-retry` (already an indirect dependency; promoted to direct here) for the exponential backoff (1s, 2s bases, both parameterized so tests inject sub-millisecond values), a `context.WithTimeout` per attempt, and `defer`-guarded per-job temp-dir removal that runs on every exit path including panic recovery.
   *Verify:* `go test ./internal/video/...` green.

3. **Real implementations (no new tests beyond an opt-in smoke test)** — `internal/video/ytdlp.go` (real `Downloader`: `exec.CommandContext(ctx, "yt-dlp", url, "-o", destPath)` — URL passed as a discrete argv element, never shell-interpolated, per spec Constraints), `internal/video/ffprobe.go` (real `Prober`: `exec.CommandContext(ctx, "ffprobe", "-print_format", "json", ...)`, parses duration/width/height/fps from JSON stdout), `internal/video/analyzer.go` (real `Analyzer`: `ffmpeg` extracts frames at a fixed sample rate — e.g. 1 fps, bounding analysis time on long videos — into the per-job temp dir as JPEGs; pure-Go `image/jpeg` decodes them; frame-to-frame pixel-diff per detected motion region produces each participant's `ActivityScore`). Deliberately **pure Go + subprocess only** — no CGO, no gocv/OpenCV bindings — so `CGO_ENABLED=0` in the Dockerfile build stays unchanged.
   A thin opt-in smoke test (`internal/video/smoke_test.go`) gated on a new `TEST_YTDLP=1` env var (same gating convention as `TEST_DSN`/`TEST_REDIS_ADDR`) exercises the real `Downloader`+`Prober`+`Analyzer` against a short fixed public test video; skipped unless that var is set (never runs in default `go test ./...`).
   *Verify:* `go build ./...` compiles. `TEST_YTDLP=1 go test ./internal/video/... ` passes locally if `yt-dlp`/`ffmpeg` are installed (optional, not required for this plan's green gate — full confirmation comes in step 8's `docker compose` pass).

4. **Red: `internal/job` result-persistence tests** — extend `internal/job/job_test.go` (TEST_DSN-gated, same convention as existing tests) with tests for two new functions: `SaveResults(ctx, db, jobID uint64, participants []video.ParticipantResult) error` (inserts one `participants` row + one `participant_metrics` row — `metric_key='activity_score'` — per participant, in a single transaction; asserts N-in/N-out including the N=0 case) and `SaveSummary(ctx, db, jobID uint64, summary string) error` (upserts the one `job_summaries` row for that job).
   *Verify:* `TEST_DSN=... go test ./internal/job/...` fails (red — functions don't exist).

5. **Green: `internal/job/job.go`** — implement `SaveResults` and `SaveSummary` using raw SQL (no ORM), reusing `participants`/`participant_metrics`/`job_summaries` exactly as defined in `001_initial_schema.sql` — no migration.
   *Verify:* `TEST_DSN=... go test ./internal/job/...` green.

6. **Red: `cmd/worker` redesigned `processTick`** — extend `cmd/worker/main_test.go` with: (a) a fake `Processor` (`Process(ctx, youtubeURL string) (video.Result, error)`) wired into `processTick`'s new signature; (b) a test asserting a `pending` job goes `pending→processing` (publish fires) then, still within the same `processTick` call, `processing→done` (second publish fires) with `SaveResults`/`SaveSummary` called, when the fake `Processor` succeeds; (c) a symmetric test for a fake `Processor` that returns a failure, asserting `processing→failed` + `SaveSummary` with a failure reason + zero `SaveResults` calls; (d) **explicitly replace** `TestProcessTick_AdvancesPendingAndProcessingSeparately` (the old stub's "one tick in processing" invariant) with a new test reflecting the new synchronous-per-job model — call out in the commit message that this is an intentional behavior change from the popup-notifications+sse stub, not an accidental regression (surfaced per CLAUDE.md's deviation rule).
   *Verify:* `go test ./cmd/worker/...` fails to compile/red.

7. **Green: `cmd/worker/main.go`** — redesign `processTick(ctx, sqlDB, redisClient, processor)`: for each snapshotted `pending` job, `UpdateStatus→processing` + publish, then call `processor.Process(ctx, j.YoutubeURL)` synchronously; on success, `job.SaveResults` + `job.SaveSummary` (success text: duration/resolution/fps/participant count) + `UpdateStatus→done` + publish; on failure, `job.SaveSummary` (failure reason) + `UpdateStatus→failed` + publish. **Remove** the old "pre-tick `processing`→`done`" loop entirely — under the new model a job's `done`/`failed` transition happens inside the same call that moved it to `processing`, so no job should ever be found sitting in `processing` at the start of a tick under normal operation (a crash mid-pipeline is the one exception, and per spec Non-scope it is deliberately never picked back up). Wire the real `video.Pipeline` (step 2/3) in `main()`, reading a new `PROCESSING_TIMEOUT` env var (default `10m`) for the per-attempt timeout.
   *Verify:* `go test ./cmd/worker/...` green.

8. **Docker/deployment** — add `RUN apk add --no-cache ffmpeg yt-dlp` to the `worker` stage of `Dockerfile` (plan B in Risks if Alpine's community repo lacks a packaged `yt-dlp`); add `PROCESSING_TIMEOUT: "10m"` to the `worker` service in `docker-compose.yml`. `api` and `web` images untouched.
   *Verify:* `docker compose up --build` succeeds; `docker compose exec worker yt-dlp --version` and `docker compose exec worker ffprobe -version` both succeed; submit one real job against a short, fixed public YouTube URL and confirm via `GET /jobs/:id` (or direct DB check) that it reaches `done` with a non-empty `job_summaries.summary` and ≥1 `participant_metrics` row — this satisfies spec acceptance criterion 9 and is UNVERIFIED (not READY) until this live pass runs, per CLAUDE.md's rule on infra/deployment criteria that unit tests can't exercise.

9. **Dependency tidy** — run `go mod tidy` to promote `github.com/sethvargo/go-retry` from indirect to direct.
   *Verify:* `go build ./...` green; `go.sum` diff shows only the direct/indirect flip, no new module versions pulled in.

10. **Full-suite verification** — `go test ./...` (with `TEST_DSN`/`TEST_REDIS_ADDR` set) all green; `golangci-lint run` clean; `go build ./cmd/...` succeeds.
    *Verify:* all three commands pass with zero errors/warnings.

## Order

Strict TDD: every implementation step (2, 3, 5, 7, 8) is preceded by its red step (1, none — see below, 4, 6, n/a) with a failing test committed first. Step 3 (real subprocess-backed implementations) has no dedicated red-phase unit test of its own — by design, per spec Scope's "testable seams," the orchestration logic (step 1/2) is what's unit-tested; step 3's real I/O is only exercised by the opt-in smoke test and step 8's live `docker compose` pass, mirroring how `specs/popup-notifications+sse` treated its nginx SSE-buffering fix as verified only by a live pass, not a unit test.

**No DB migration step.** Per spec Constraints, this feature reuses `participants`/`participant_metrics`/`job_summaries`/`analysis_jobs` exactly as defined in `001_initial_schema.sql` — no schema change, so no migration step/commit exists in this plan. (If review surfaces a need for one — e.g., to store failure-reason structure more richly than free text — that would be a plan amendment, not a silent addition.)

**Amendment (surfaced after step 1/4/6 were drafted, before any commit):** this plan originally stated red-phase test commits and green-phase implementation commits would be separate, reading CLAUDE.md's "tests ship in the same commit as their implementation" as applying only "at the feature level." That reading was wrong — the rule is literal: a test and the code that makes it pass land in one commit, never two. Corrected process: each red-phase step's tests are written and verified failing (red), then held **uncommitted** until the paired implementer step turns them green, at which point test + implementation are committed together as a single commit. TDD ordering (tests written first, red confirmed) is preserved; only the commit boundary changes.

## Codegen

N/A — this stack has no code generation tooling (no sqlc, protobuf, or OpenAPI generation anywhere in the repo). No regeneration step is needed.

## Risks

1. **Alpine's community repo may not carry a packaged `yt-dlp`.** Plan B: install via `apk add --no-cache python3 py3-pip && pip install --break-system-packages yt-dlp` in the worker stage instead. Step 8 checks this first and falls back if `apk add yt-dlp` fails.
2. **Behavior change to `TestProcessTick_AdvancesPendingAndProcessingSeparately`.** The old "exactly one tick in processing" invariant from the popup-notifications+sse stub is intentionally replaced (step 6/7), not accidentally broken. Flagged explicitly here and in the step 6 commit message so it's never only discoverable via `git log`.
3. **Pure-Go frame decoding could be slow on long/high-resolution videos**, risking the per-attempt timeout on legitimate videos. Mitigation: fixed low frame-sample rate (1 fps) bounds decode work independent of video length/resolution; the per-attempt timeout (step 7) is the backstop, and criterion 8 in the spec covers the case where even that isn't enough (ends in `failed` with a timeout-specific reason, not a silent hang).
4. **Real backoff durations (1s/2s) would make the test suite slow if not injectable.** Mitigated in step 1/2 by parameterizing the backoff base — tests pass sub-millisecond values, production wires 1s.
5. **Command injection via `exec.Command`.** Mitigated by construction (step 3: URL always passed as a discrete argv element, never through a shell) and explicitly checked by the reviewer stage against spec Constraints.
6. **Disk-space leak if the worker is `SIGKILL`ed (not `SIGTERM`) mid-pipeline.** Accepted risk — `defer`-based cleanup cannot run past a hard kill; this matches the existing worker's graceful-shutdown model (`signal.NotifyContext` on SIGTERM/SIGINT only) and is not something this plan can close.
7. **`yt-dlp` itself breaking over time** as YouTube changes formats/APIs is an ongoing external-dependency risk with no code-level mitigation in this plan; noted as an operational concern for `/harness:retro` or a future spec, not something to solve here.

## Out of scope guard

Do not touch:
- `web/` — no frontend/UI changes; spec Non-scope forbids any change to `POST /jobs`/`GET /jobs`/`GET /jobs/:id` request/response shapes.
- `internal/handler/` — no new HTTP endpoints.
- `internal/otp/`, `internal/session/`, `internal/user/` — authentication is untouched.
- `internal/db/migrations/` — no new migration; schema reuse only.
- `job.validTransitions` and the `status` column's valid values in `internal/job/job.go` — unchanged (`pending→processing`, `processing→done/failed`).
- Any ML/CV dependency (gocv, OpenCV bindings, MediaPipe, PyTorch, ONNX runtime) — pure Go + `yt-dlp`/`ffmpeg` subprocess only, per spec Non-scope.
- `docker-compose.yml`'s `api`, `web`, `mysql`, `redis` service definitions — only the `worker` service gains the new `PROCESSING_TIMEOUT` env var.
