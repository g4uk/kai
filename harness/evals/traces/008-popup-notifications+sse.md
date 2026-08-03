# 008: popup-notifications+sse — real-time job-status popups via SSE + Redis pub/sub

## Prompt

Add real-time job-status popup notifications to kumite-analyzer, delivered via
Server-Sent Events. Add `internal/job.UpdateStatus` (a guarded status-transition
writer accepting only `pending→processing`, `processing→done`, and
`processing→failed`; any other transition is rejected, the row is not written,
and nothing is published) and replace the worker's no-op idle loop with logic
that mechanically advances `pending` jobs to `processing` then to `done` on a
fixed timer (a deliberate stand-in for the real analysis pipeline, not real
video processing) — publishing each transition to a single Redis pub/sub
channel. Add a new `GET /jobs/stream` SSE endpoint behind the existing session
middleware: the api process holds one Redis subscription and fans out events
in-process to open connections, filtered strictly by the connection's own
authenticated `user_id` (never a client-supplied one) so one user's job status
changes are never visible to another user's connection, and periodically
re-validates the session on long-lived connections since the one-time
middleware check isn't enough for a stream that can outlive a session's TTL.
Fix `web/nginx.conf` to disable proxy buffering so SSE events flush
incrementally instead of being held until the connection closes. On the
frontend, add a dismissible toast/popup UI primitive and a hook that opens an
`EventSource` to the new endpoint while the user is authenticated, shows a
popup with status-appropriate text on each event (visibly different for a
terminal `done`/`failed` status than for `processing`), and closes the
connection on logout.

## Checks

- [ ] cmd: go test ./... -race -count=1
- [ ] cmd: go test ./internal/jobevents/... -run "TestBroadcaster_DispatchIsolatesOtherUsers" -v 2>&1 | grep -F -- "--- PASS: TestBroadcaster_DispatchIsolatesOtherUsers"
- [ ] cmd: cd web && npm ci && OUT=$(npx vitest run --no-color src/features/notifications/useJobStatusEvents.test.tsx -t "distinguishes terminal" 2>&1); echo "$OUT"; echo "$OUT" | grep -qE '[1-9][0-9]* passed'
- [ ] cmd: docker compose up --build -d && sleep 30 && PHONE="+1555$(date +%N | cut -c1-7)" && curl -s -o /dev/null -w '%{http_code}' -X POST http://localhost:8081/api/auth/otp/request -d "{\"phone_number\":\"$PHONE\"}" | grep -q '^202$' && OTP=$(docker compose logs api 2>/dev/null | grep -F "$PHONE" | grep -oE 'code=[0-9]{6}' | tail -1 | cut -d= -f2) && curl -s -c /tmp/sse-eval-cookie.txt -o /dev/null -X POST http://localhost:8081/api/auth/otp/verify -d "{\"phone_number\":\"$PHONE\",\"code\":\"$OTP\"}" && (timeout 100 curl -N -s -b /tmp/sse-eval-cookie.txt http://localhost:8081/api/jobs/stream > /tmp/sse-eval-out.txt &) && sleep 1 && curl -s -o /dev/null -b /tmp/sse-eval-cookie.txt -X POST http://localhost:8081/api/jobs -d '{"youtube_url":"https://www.youtube.com/watch?v=jNQXAC9IVRw"}' && sleep 90 && docker compose logs worker && [ "$(grep -c '\"status\":\"processing\"' /tmp/sse-eval-out.txt)" = "1" ] && [ "$(grep -c '\"status\":\"done\"' /tmp/sse-eval-out.txt)" = "1" ]
- [ ] (manual) if the last check ever fails only on nginx buffering specifically (events never arrive incrementally, or arrive all at once right as the 100s timeout kills the connection), check web/nginx.conf's /api/ location still has proxy_buffering off — that's the one thing this check can't cheaply isolate from a general SSE-pipeline failure
