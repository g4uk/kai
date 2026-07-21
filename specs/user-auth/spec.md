# Spec: User Auth

## Problem

The API currently has no way to identify who is making a request — every future handler needs `user_id` scoping (per CLAUDE.md's hard rule), but there is no login mechanism and the `users` table has no credential. Coaches and athletes need a low-friction way to sign in without managing passwords, so the product authenticates by phone number + one-time passcode (OTP), auto-provisioning an account on a phone number's first successful verification.

## Scope

- `POST /auth/otp/request` — accepts a phone number, generates a 6-digit OTP, stores it (hashed) in Redis with a 5-minute TTL, and logs it to stdout (stub in place of real SMS delivery)
- `POST /auth/otp/verify` — accepts a phone number + OTP code; on match, looks up the user by phone number or creates one if none exists, creates a server-side session in Redis, and sets a session cookie
- `POST /auth/logout` — invalidates the caller's session in Redis and clears the session cookie
- Session-validation middleware: resolves the session cookie to a `user_id` via Redis lookup and attaches it to the request context; rejects missing/invalid/expired sessions with `401`
- Migration: add a unique, indexed `phone_number` column to `users`; relax `email` to nullable (phone-only accounts have no email)
- Per-phone-number rate limiting on OTP request and OTP verify attempts, enforced via Redis counters
- OTP codes are single-use: invalidated immediately after a successful verify, after exceeding the attempt limit, or after their TTL expires — whichever comes first

## Non-scope

- Real SMS delivery / third-party SMS provider integration (explicitly stubbed to a log line for this spec)
- Password-based, magic-link, or OAuth authentication
- Email verification or email-based communication of any kind
- Admin-created accounts, invites, or any account-provisioning path other than first successful OTP verify
- Changing/updating a phone number on an existing account, or merging two accounts
- Roles, permissions, or an admin panel
- Account deletion or deactivation
- Multi-factor authentication beyond the single OTP step
- Authorization/ownership checks on non-auth resources (e.g., `analysis_jobs` ownership) — those are enforced per-feature per CLAUDE.md's hard rule, not by this spec

## Acceptance criteria

1. When `POST /auth/otp/request` is called with a valid E.164 phone number, then a 6-digit numeric OTP is generated, its hash is stored in Redis with a 5-minute TTL, and the plaintext code is logged to stdout; the response is `202` with no OTP value in the body.
2. When `POST /auth/otp/request` is called with a malformed phone number (not E.164), then the response is `400` and no OTP is generated or stored.
3. When `POST /auth/otp/verify` is called with the correct code for a phone number within its 5-minute TTL, then the response is `200`, a session is created in Redis, and a `Set-Cookie` header with an `HttpOnly`, `Secure`, `SameSite=Strict` session cookie is returned.
4. When `POST /auth/otp/verify` succeeds for a phone number that has no existing `users` row, then exactly one new `users` row is created with that `phone_number` before the session is issued.
5. When `POST /auth/otp/verify` succeeds for a phone number that already has a `users` row, then no new row is created and the session's `user_id` matches the existing row.
6. When `POST /auth/otp/verify` is called with an incorrect code, then the response is `401`, no session is created, and the stored attempt counter for that OTP increments by 1.
7. When `POST /auth/otp/verify` is called with a code after its 5-minute TTL has elapsed, then the response is `401` (`otp_expired`), and no session is created.
8. When `POST /auth/otp/verify` succeeds once for a given OTP, then a second verify call with the same code — even within the TTL — returns `401` (already used).
9. When a 6th consecutive incorrect verify attempt is made against the same OTP (attempt limit = 5), then the OTP is invalidated immediately, and the response is `401` (`too_many_attempts`) even if the TTL has not expired.
10. When `POST /auth/otp/request` is called more than 5 times for the same phone number within a rolling 1-hour window, then the 6th+ request returns `429` and no new OTP is generated.
11. When `POST /auth/otp/request` is called again for a phone number that already has a live (unexpired, unused) OTP, then the prior OTP is invalidated and only the newest code verifies successfully.
12. When `POST /auth/logout` is called with a valid session cookie, then the session is deleted from Redis, the response is `200`, and the cookie is cleared (`Max-Age=0`).
13. When any endpoint protected by the session middleware is called without a session cookie, then the response is `401`.
14. When any endpoint protected by the session middleware is called with a session cookie that does not resolve to a live Redis session (expired, logged out, or unknown), then the response is `401`.
15. When `go build ./cmd/...` is run, the api binary compiles with the new endpoints and middleware with zero errors.

## Edge cases

1. **New phone number, valid OTP** — auto-creates a `users` row and establishes a session in one request (criterion 4).
2. **Existing phone number, valid OTP** — reuses the existing `user_id`; no duplicate row (criterion 5).
3. **Expired OTP presented** — `401 otp_expired`, no session (criterion 7).
4. **OTP already consumed, replayed** — second verify with the same code fails even inside the TTL window (criterion 8).
5. **Verify-attempt flood on one OTP** — 6th wrong guess invalidates the code early, independent of TTL (criterion 9).
6. **Request flood for one phone number** — 6th request within the hour is rejected before any OTP is generated (criterion 10).
7. **Overlapping OTP requests for the same phone number** — requesting a new code invalidates the previous one; only the latest is valid (criterion 11).
8. **Stale/foreign session cookie replay** — a cookie value for a session that was logged out, expired, or never existed in Redis is rejected with `401`, never silently treated as anonymous or attributed to a different `user_id`.
9. **Malformed phone number format** — rejected at request time with `400`, never reaches OTP generation or storage.

## Constraints

- Security: OTP codes are stored in Redis as a salted hash, never in plaintext, except the stdout stub log line used to stand in for SMS delivery (must be clearly marked as a temporary stand-in, not a production log of user secrets).
- Security: session cookie is `HttpOnly`, `Secure`, `SameSite=Strict`; the raw session token is never logged.
- Security: rate limits (5 OTP requests/hour/phone, 5 verify attempts/OTP) are enforced server-side via Redis and cannot be bypassed by omitting or spoofing client-side state.
- Performance: `/auth/otp/verify` and session-middleware lookups must complete in under 200 ms under normal (all-up) conditions, consistent with the existing `/healthz` latency bar.
- Compatibility: the `users.email` column becomes nullable via an additive migration; no existing rows are dropped or backfilled with placeholder data.
- Compatibility: session storage uses Redis (already part of the stack per decision 002); no new stateful service is introduced.
- No ORM — raw SQL via `database/sql`/`sqlx` for the `users` migration and lookup, per CLAUDE.md.
- Every query scoped by `user_id` once a session resolves it, per CLAUDE.md's hard rule — applies to every handler built on top of this spec, not just auth endpoints themselves.
