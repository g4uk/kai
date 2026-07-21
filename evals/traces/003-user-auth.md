# 003: user-auth — phone + OTP authentication
## Prompt
Implement phone-number + one-time-passcode (OTP) authentication for the API: a
`POST /auth/otp/request` endpoint that generates a 6-digit OTP for a given
E.164 phone number, stores a salted hash of it in Redis with a 5-minute TTL,
and logs the plaintext code to stdout as a stand-in for real SMS delivery; a
`POST /auth/otp/verify` endpoint that checks the submitted code, auto-creates
a `users` row on a phone number's first successful verify (no separate
registration flow), and issues a server-side session (Redis-backed, 30-day
TTL) via an `HttpOnly`/`Secure`/`SameSite=Strict` cookie; a `POST /auth/logout`
endpoint that deletes the session and clears the cookie; and session-
validation middleware that resolves the cookie to a `user_id` for future
protected routes. Both OTP requests and OTP verify attempts are rate-limited
per phone number via Redis counters (5 requests/hour, 5 attempts/code), and
OTP codes are single-use (invalidated on success, on hitting the attempt
limit, or on TTL expiry, whichever comes first).
## Checks
- [ ] cmd: go test ./... -count=1
- [ ] cmd: docker compose up --build -d && sleep 30 && curl -s -o /dev/null -w '%{http_code}' -X POST http://localhost:8080/auth/otp/request -d '{"phone_number":"not-a-number"}' | grep -q '^400$' && curl -s -o /dev/null -w '%{http_code}' -X POST http://localhost:8080/auth/otp/request -d '{"phone_number":"+15550001111"}' | grep -q '^202$' && (for i in 1 2 3 4 5 6; do curl -s -o /dev/null -w '%{http_code}\n' -X POST http://localhost:8080/auth/otp/request -d '{"phone_number":"+15559998888"}'; done | tail -1 | grep -q '^429$')
- [ ] (manual) set TEST_DSN to a real MySQL 8.0 DSN and TEST_REDIS_ADDR to a real Redis 7 addr to run the integration tests locally instead of skipping them
