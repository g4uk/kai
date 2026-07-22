# Plan: Auth Me

Source: `specs/auth-me/spec.md`

Conventions confirmed from the codebase: same as `specs/user-auth/plan.md` — stdlib `net/http` `ServeMux`, thin handlers, plain `testing` table-driven tests, no ORM. This is the smallest possible addition: a zero-dependency handler behind the existing `SessionMiddleware`.

## Steps

1. **`internal/handler/auth.go` — `MeHandler`**
   - Files: `internal/handler/auth.go`, `internal/handler/auth_test.go`
   - What: `MeHandler struct{}` with `ServeHTTP` that writes `http.StatusNoContent` and nothing else — no fields, no interfaces, since reaching the handler at all (past `SessionMiddleware`) is the only fact it reports.
   - Test first (red): table-driven case in `auth_test.go` — wrap `MeHandler` in `SessionMiddleware` with a stub `SessionValidator`: valid session → `204`, empty body; invalid/missing session → `401` (proves `SessionMiddleware`'s existing behavior, not new logic in `MeHandler`).
   - Implement until green.
   - Verify: `go test ./internal/handler/...`.

2. **Wire into `cmd/api/main.go`**
   - Files: `cmd/api/main.go`, `cmd/api/main_test.go`
   - What: `mux.Handle("GET /auth/me", handler.SessionMiddleware(&handler.MeHandler{}, sessionValidator))` — `sessionValidator` already exists in `buildServer`'s signature (accepted but previously unused per a NOTE comment in `main.go`; this is its first real consumer).
   - Test first (red): extend the existing "routes registered" test — unauthenticated request to `GET /auth/me` returns `401`; authenticated (stub session) returns `204`.
   - Implement until green.
   - Verify: `go build ./cmd/...` && `go test ./...`.

3. **Frontend: swap the `ui` feature's auth probe from `GET /jobs` to `GET /auth/me`**
   - Files: `web/src/api/client.ts`, `web/src/api/client.test.ts`, `web/src/app/AuthContext.tsx`, `web/src/app/router.test.tsx`
   - What: add `getAuthMe(): Promise<void>` to the API client (resolves on `204`, rejects with `ApiError` on `401` — reuses the existing `apiFetch`/`ApiError`/`onUnauthorized` machinery, no new error handling). `AuthContext`'s mount-time probe switches from calling `listJobs()` to `getAuthMe()` — success ⇒ `"authenticated"`, any error ⇒ `"anonymous"`. The job-list screen's own `useJobs` hook already calls `listJobs()` independently when it mounts, so removing the probe's side-effect of "pre-fetching" jobs costs nothing — it just means `/jobs` is fetched once, by the screen that actually needs it, instead of twice.
   - Test first (red): `client.test.ts` — `getAuthMe` resolves on `204`, rejects with `ApiError{status:401}` on `401`. `router.test.tsx`'s existing criterion-1 test (`"an unauthenticated root load shows the login screen and probes listJobs exactly once, with no getJob call"`) gets rewritten to assert `getAuthMe` is called and **`listJobs` is never called** — this is the literal fix for criterion 1's "no `/jobs` request is made," now provable rather than a documented exception.
   - Implement until green.
   - Verify: `cd web && npm test -- --run`.

4. **Re-verify `specs/ui/spec.md` criterion 1**
   - Re-run the acceptance-criteria check for AC-1 only: confirm the rewritten `router.test.tsx` test passes and asserts zero `listJobs`/`getJob` calls on an unauthenticated root load.
   - Update `specs/ui/plan.md`'s Risks section to remove the now-resolved "No `/auth/me` endpoint exists" entry (or mark it resolved-by-`specs/auth-me`), so the plan doesn't keep describing a workaround that no longer exists.

## Order

Step 1 (Go handler) before step 2 (wiring) before step 3 (frontend swap, which depends on the endpoint existing) before step 4 (re-verify, no new code). No migration — no schema change.

## Codegen

Not applicable — no codegen tooling in this repo.

## Risks

- **Tiny surface, low risk.** This is about as small as a backend change gets (one no-op handler, one route). The only real risk is forgetting to actually remove the `listJobs`-as-probe behavior in step 3, which would leave two `/jobs`-adjacent calls firing (the new `/auth/me` probe plus the old workaround) — step 3's test explicitly asserts `listJobs` is *not* called during the probe to guard against that.

## Out of scope guard

Do not touch: `internal/otp/*`, `internal/session/*`, `internal/user/*`, `internal/job/*`, `internal/db/migrations/*` — no changes needed anywhere except the one new route and the frontend probe swap.
