# 002: walking-skeleton — boot the full stack from scratch
## Prompt
Implement the walking skeleton for a Go project with no existing source code.
Build: go.mod (module github.com/g4uk/kai), two binaries (cmd/api on :8080
and cmd/worker), a Docker Compose stack with four services (api, worker,
mysql:8.0, redis:7-alpine), and a Goose migration for five tables (users,
analysis_jobs, participants, participant_metrics, job_summaries). The api must
serve GET /healthz returning JSON {"status","mysql","redis"} — HTTP 200 when
both dependencies are healthy, HTTP 503 with per-dependency error fields when
either is down. Both binaries must retry connections with exponential back-off
(100ms initial, ×2, cap 5s, 30s budget for api). No auth, no endpoints beyond
/healthz, no business logic. docker compose up --build must reach all-healthy
within 60 seconds.
## Checks
- [ ] cmd: go test ./...
- [ ] cmd: docker compose up --build -d && sleep 30 && curl -sf http://localhost:8080/healthz | grep '"status":"ok"'
- [ ] cmd: docker compose stop mysql && curl -s http://localhost:8080/healthz | python3 -c "import sys,json; d=json.load(sys.stdin); assert d['mysql']=='error' and d['redis']=='ok' and d['status']=='error'"
- [ ] cmd: docker compose down -v && docker compose up --build -d && sleep 30 && docker compose exec mysql mysql -uroot -psecret kumite -e "SHOW TABLES;" 2>/dev/null | grep -c '\w' | grep -qE '^[56]$'
