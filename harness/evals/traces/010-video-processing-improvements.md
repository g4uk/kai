# 010: video-processing-improvements — progress events, download validation, tunable thresholds, top-2 fighter selection

## Prompt

Improve the real video-processing pipeline shipped in `specs/video-processing`
along five axes surfaced by running it against real kumite footage. Report
which pipeline stage (downloading/probing/analyzing) a job is in via a new
ephemeral `jobevents.StageChanged` event published alongside the existing
`pending`/`processing`/`done`/`failed` status events, without changing
`job.status` or its valid transitions. Validate a downloaded file is
non-empty and passes a basic ffprobe integrity check before analysis begins,
treating a failure as a new, retryable error category distinct from the
existing (deterministic, non-retryable) too-short-video case. Promote the
analyzer's hardcoded motion-detection grid/threshold/min-region constants to
`FFMPEGAnalyzer` struct fields configurable via new optional env vars,
defaulting to today's exact values when unset. Most importantly, stop
treating every detected moving region as a participant — kumite footage
always has an active referee and often corner judges or a moving crowd in
frame — by ranking detected regions by a persistence score (the fraction of
frame-pairs each region stayed active in, since fighters move near-
continuously while a referee's motion is comparatively bursty) and keeping
only the top `MaxParticipants` (default 2, configurable) regions. Finally,
categorize processing failures into a small fixed set of human-readable
reasons instead of writing the raw wrapped Go error (which can include
internal temp-directory paths and raw subprocess stderr) into
`job_summaries.summary`, while still logging the complete unredacted error
server-side for operator debugging.

## Checks

- [ ] cmd: go test ./... -count=1
- [ ] cmd: go test ./internal/video/... -run "TestDetectParticipants_TopNByPersistenceBeatsRawAccumulatedSum" -v 2>&1 | grep -F -- "--- PASS: TestDetectParticipants_TopNByPersistenceBeatsRawAccumulatedSum"
- [ ] cmd: go test ./cmd/worker/... -run "TestFailureSummary_CategorizesWithoutLeakingRawErrorText" -v 2>&1 | grep -F -- "--- PASS: TestFailureSummary_CategorizesWithoutLeakingRawErrorText"
- [ ] cmd: docker compose up --build -d && sleep 30 && PHONE="+1555$(date +%N | cut -c1-7)" && curl -s -o /dev/null -w '%{http_code}' -X POST http://localhost:8081/api/auth/otp/request -d "{\"phone_number\":\"$PHONE\"}" | grep -q '^202$' && OTP=$(docker compose logs api 2>/dev/null | grep -F "$PHONE" | grep -oE 'code=[0-9]{6}' | tail -1 | cut -d= -f2) && curl -s -c /tmp/vpi-eval-cookie.txt -o /dev/null -X POST http://localhost:8081/api/auth/otp/verify -d "{\"phone_number\":\"$PHONE\",\"code\":\"$OTP\"}" && (timeout 60 curl -N -s -b /tmp/vpi-eval-cookie.txt http://localhost:8081/api/jobs/stream > /tmp/vpi-eval-sse.txt &) && sleep 1 && JOB=$(curl -s -b /tmp/vpi-eval-cookie.txt -X POST http://localhost:8081/api/jobs -d '{"youtube_url":"https://www.youtube.com/watch?v=jNQXAC9IVRw"}') && ID=$(echo "$JOB" | grep -oE '"id":[0-9]+' | grep -oE '[0-9]+') && for i in $(seq 1 20); do R=$(curl -s -b /tmp/vpi-eval-cookie.txt http://localhost:8081/api/jobs/$ID); echo "$R" | grep -q '"status":"done"' && break; echo "$R" | grep -q '"status":"failed"' && break; sleep 5; done && docker compose logs worker && echo "$R" | grep -q '"status":"done"' && echo "$R" | grep -q 'activity_score' && grep -q 'event: job_stage' /tmp/vpi-eval-sse.txt
- [ ] (manual) the live check above only proves pipeline mechanics + stage-event delivery end-to-end against a single-participant test video (`jNQXAC9IVRw`, same stable ID used in 009's trace) — it cannot durably assert "exactly 2 fighters, referee excluded" against real match footage, since that requires a curated real video (URL-rot and content-dependent-accuracy risk, matching spec's own Constraints that the persistence heuristic isn't guaranteed 100% accurate). Confirming the referee-exclusion behavior against a real match clip is a manual/periodic check, not something to hardcode into this CI trace.
