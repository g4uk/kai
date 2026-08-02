# 009: video-processing — real download/detect/score pipeline replacing the worker stub

## Prompt

Replace the worker's fixed-delay stub with a real per-job pipeline: download the
submitted YouTube video (yt-dlp/ffmpeg, added to the worker Docker image),
extract technical metadata, auto-detect participants via a lightweight
motion-region pass over sampled frames (no ML/pose estimation), and compute a
frame-difference "activity score" per participant, writing exactly one
`participants`/`participant_metrics` row per detected participant and one
`job_summaries` row (success text or failure reason) — reusing the existing
schema, with no new migration or job status. A processing attempt (download →
metadata → detect → score) is retried up to 3 total attempts with exponential
backoff (1s, 2s) for transient failures, but a deterministic failure (e.g. too
few usable frames) fails immediately on attempt 1 without wasting retries. Each
attempt is bounded by a configurable timeout and its per-job temp directory is
always cleaned up (success, failure, or timeout). The whole download/probe/
detect pipeline sits behind small consumer-defined interfaces so it's testable
with fakes, with zero real network/subprocess calls in the default test suite.

## Checks

- [ ] cmd: go test ./... -count=1
- [ ] cmd: go test ./internal/video/... -run "TestPipeline_Run$" -v 2>&1 | grep -F -- "--- PASS: TestPipeline_Run/transient_failure_then_success"
- [ ] cmd: go test ./internal/video/... -run "TestPipeline_Run_NonRetryableAnalyzeErrorFailsOnFirstAttempt" -v 2>&1 | grep -F -- "--- PASS: TestPipeline_Run_NonRetryableAnalyzeErrorFailsOnFirstAttempt"
- [ ] cmd: docker compose up --build -d && sleep 30 && docker compose exec -T worker yt-dlp --version | grep -qvE '^2023\.' && PHONE="+1555$(date +%N | cut -c1-7)" && curl -s -o /dev/null -w '%{http_code}' -X POST http://localhost:8081/api/auth/otp/request -d "{\"phone_number\":\"$PHONE\"}" | grep -q '^202$' && OTP=$(docker compose logs api 2>/dev/null | grep -F "$PHONE" | grep -oE 'code=[0-9]{6}' | tail -1 | cut -d= -f2) && curl -s -c /tmp/vp-eval-cookie.txt -o /dev/null -X POST http://localhost:8081/api/auth/otp/verify -d "{\"phone_number\":\"$PHONE\",\"code\":\"$OTP\"}" && JOB=$(curl -s -b /tmp/vp-eval-cookie.txt -X POST http://localhost:8081/api/jobs -d '{"youtube_url":"https://www.youtube.com/watch?v=jNQXAC9IVRw"}') && ID=$(echo "$JOB" | grep -oE '"id":[0-9]+' | grep -oE '[0-9]+') && for i in $(seq 1 20); do R=$(curl -s -b /tmp/vp-eval-cookie.txt http://localhost:8081/api/jobs/$ID); echo "$R" | grep -q '"status":"done"' && break; echo "$R" | grep -q '"status":"failed"' && break; sleep 5; done && docker compose logs worker && echo "$R" | grep -q '"status":"done"' && echo "$R" | grep -qE '"summary":"[^"]+' && echo "$R" | grep -q 'activity_score'
- [ ] (manual) if the last check fails specifically on the yt-dlp version grep, Alpine's packaged yt-dlp may have drifted stale again — re-check Dockerfile still pip-installs a current release rather than relying on `apk add yt-dlp`
