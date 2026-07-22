# 004: jobs-api — job submission, listing, and results endpoints
## Prompt
Implement three endpoints for submitting and retrieving video-analysis jobs,
behind the existing OTP session auth: `POST /jobs` accepts a `youtube_url`,
validates it against accepted YouTube URL shapes (`watch?v=`, `youtu.be/`,
`/shorts/`), creates an `analysis_jobs` row scoped to the authenticated user
with `status='pending'`, and returns `409` if the user already has a
non-failed job for that exact URL; `GET /jobs` lists the authenticated user's
own jobs, newest-first, with no pagination; `GET /jobs/:id` returns a single
job's fields plus its participants (each with metrics as key/value pairs) and
summary, scoped to the authenticated user — a job belonging to another user
or that doesn't exist returns an identical `404`. All three run behind the
existing session middleware (401 without a valid session), use raw SQL (no
ORM), and never enqueue to Redis or perform any analysis — those are
out of scope for this feature.
## Checks
- [ ] cmd: go test ./... -count=1
- [ ] cmd: docker compose up --build -d && sleep 30 && curl -s -o /dev/null -w '%{http_code}' -X POST http://localhost:8080/jobs -d '{"youtube_url":"https://www.youtube.com/watch?v=dQw4w9WgXcQ"}' | grep -q '^401$'
- [ ] cmd: docker compose up --build -d && sleep 30 && curl -s -o /dev/null -w '%{http_code}' -X POST http://localhost:8080/auth/otp/request -d '{"phone_number":"+15551230001"}' | grep -q '^202$' && OTP=$(docker compose logs api 2>/dev/null | grep -F '+15551230001' | grep -oE 'code=[0-9]{6}' | tail -1 | cut -d= -f2) && curl -s -c /tmp/jc.txt -o /dev/null -X POST http://localhost:8080/auth/otp/verify -d "{\"phone_number\":\"+15551230001\",\"code\":\"$OTP\"}" && JOB=$(curl -s -b /tmp/jc.txt -X POST http://localhost:8080/jobs -d '{"youtube_url":"https://www.youtube.com/watch?v=dQw4w9WgXcQ"}') && echo "$JOB" | python3 -c "import sys,json; d=json.load(sys.stdin); assert d['status']=='pending'" && curl -s -b /tmp/jc.txt -o /dev/null -w '%{http_code}' -X POST http://localhost:8080/jobs -d '{"youtube_url":"https://www.youtube.com/watch?v=dQw4w9WgXcQ"}' | grep -q '^409$' && curl -s -b /tmp/jc.txt http://localhost:8080/jobs | python3 -c "import sys,json; d=json.load(sys.stdin); assert len(d['jobs'])==1" && ID=$(echo "$JOB" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])") && curl -s -b /tmp/jc.txt http://localhost:8080/jobs/$ID | python3 -c "import sys,json; d=json.load(sys.stdin); assert d['participants']==[]"
- [ ] cmd: docker compose up --build -d && sleep 30 && curl -s -o /dev/null -w '%{http_code}' -X POST http://localhost:8080/auth/otp/request -d '{"phone_number":"+15551230002"}' | grep -q '^202$' && OTP=$(docker compose logs api 2>/dev/null | grep -F '+15551230002' | grep -oE 'code=[0-9]{6}' | tail -1 | cut -d= -f2) && curl -s -c /tmp/jc2.txt -o /dev/null -X POST http://localhost:8080/auth/otp/verify -d "{\"phone_number\":\"+15551230002\",\"code\":\"$OTP\"}" && curl -s -b /tmp/jc2.txt -o /dev/null -w '%{http_code}' -X POST http://localhost:8080/jobs -d '{"youtube_url":"https://example.com/not-youtube"}' | grep -q '^400$' && curl -s -b /tmp/jc2.txt -o /dev/null -w '%{http_code}' http://localhost:8080/jobs/999999999 | grep -q '^404$'
- [ ] (manual) set TEST_DSN to a real MySQL 8.0 DSN to run internal/job's integration tests (including the foreign-tenant-404 case, TestGetByID_ForeignTenant) locally instead of skipping them
