# Plan: Session Revalidation

Source: `specs/session-revalidation/spec.md`

Conventions confirmed from the codebase: frontend-only change, same conventions as `specs/auth-me/plan.md` step 3 and `specs/ui/plan.md` step 4 — Vitest + Testing Library, tests live in `web/src/app/router.test.tsx` (the existing home for all `AuthContext`/`ProtectedRoute`/routing behavior, per that file's own header comment), `../api/client` mocked via `vi.mock`. No new files needed — this is a small, surgical change to one hook.

Investigation finding (drives the whole plan): `ProtectedRoute`'s `useEffect(() => { ensureChecked(); }, [ensureChecked])` in `web/src/app/ProtectedRoute.tsx` **already fires on every mount** — each protected `<Route>` in `router.tsx` wraps its own `<ProtectedRoute>` instance, so navigating `/jobs` → `/jobs/123` unmounts/remounts it. The only thing suppressing revalidation today is `ensureChecked`'s internal guard in `AuthContext.tsx`: `if (status !== "unknown" || checking.current) return;`. **`ProtectedRoute.tsx` itself needs no code changes** — its existing `status === "unknown"` (block) / `"anonymous"` (redirect) / else (render) branching already produces the exact optimistic behavior the spec asks for, once `AuthContext` actually re-probes. This is called out explicitly (not silently omitted) because `specs/session-revalidation/spec.md`'s Scope section names `ProtectedRoute.tsx` directly.

## Steps

1. **`AuthContext.tsx` — re-probe on every mount, with 401-only demotion on revalidation**
   - Files: `web/src/app/AuthContext.tsx`, `web/src/app/router.test.tsx`
   - What:
     - Import `ApiError` from `../api/client` (already exported, used elsewhere via `client.ts`'s own tests).
     - Change `ensureChecked`'s guard from `if (status !== "unknown" || checking.current) return;` to `if (status === "anonymous" || checking.current) return;` — so it still runs once for `"unknown"` (first load, unchanged) **and now also** for `"authenticated"` (revalidation on every subsequent mount), but still skips `"anonymous"` (already redirecting — a probe there is wasted work) and skips while a probe is already in flight (single global in-flight guard — see Risks for why this is sufficient to prevent races without extra request-ID bookkeeping).
     - Capture `const wasAuthenticated = status === "authenticated";` before firing the probe.
     - On success: `setStatus("authenticated")` (unchanged — a no-op re-render when already authenticated).
     - On failure: if `wasAuthenticated` and the error is *not* a `401` `ApiError` (i.e. `!(err instanceof ApiError && err.status === 401)`), do nothing — leave `status` as `"authenticated"` (spec edge case 2: a network hiccup or 5xx during revalidation must not force a logout). Otherwise (first-load `"unknown"` path, unchanged; or revalidation that got a real `401`), `setStatus("anonymous")`.
   - Test first (red), added to `router.test.tsx`:
     a. *(criterion 1, 4)* Render at `/jobs` (authenticated), wait for the job list, assert `getAuthMe` called once. Click a test-only nav trigger to `/jobs/123`, wait for the results screen, assert `getAuthMe` now called **twice**.
     b. *(criterion 2, 4 — optimistic render)* Same navigation, but the second `getAuthMe` call returns a controllable pending promise. Assert the destination screen is already rendered *before* that promise resolves (content isn't gated on the probe).
     c. *(criterion 3)* Continuing (b), reject the pending promise with `ApiErrorMock(401, "unauthorized")`; assert the SPA redirects to `/login`.
     d. *(edge case 2)* Same setup as (a), but the second `getAuthMe` call rejects with a plain `Error("network down")` (not an `ApiError`, and separately a case with `ApiErrorMock(500, "server error")`); assert the destination screen stays rendered and no redirect to `/login` occurs.
     e. *(edge case 1 — overlap guard)* First `getAuthMe` call (triggered by an authenticated mount's revalidation) is left pending; before it resolves, remount/renavigate again; assert only one `getAuthMe` call is outstanding (the guard skips the second attempt rather than firing a concurrent, potentially out-of-order call).
   - Implement until green.
   - Verify: `cd web && npm test -- --run`.

2. **Re-verify unchanged paths (criterion 5, edge cases 3 & 5) and update doc comments**
   - Files: `web/src/app/AuthContext.tsx`, `web/src/app/ProtectedRoute.tsx` (comments only), `web/src/app/router.test.tsx` (no new assertions, just confirm)
   - What: re-run the existing first-load and deep-link-while-logged-out tests (`router.test.tsx`'s criterion-1 and edge-case-8 tests) to confirm the `"unknown"` path's blocking/redirect behavior is byte-for-byte unchanged. Update the header comments on `AuthContext.tsx` (currently describes only the mount-time probe as one-shot) and `ProtectedRoute.tsx` (currently says "triggers a probe" without noting it now re-fires on every mount) to reference `specs/session-revalidation/spec.md`, so the doc comments don't keep describing the pre-revalidation behavior as current.
   - No production logic changes in this step — comments and verification only.
   - Verify: `cd web && npm test -- --run` (full suite, confirming zero regressions across `JobListPage.test.tsx`, `NewJobPage.test.tsx`, `JobResultsPage.test.tsx`, `router.test.tsx`, `client.test.ts`).

## Order

Step 1 (core logic + its own tests, one commit) before step 2 (doc comments + full-suite re-verification, no logic change). No DB migration — this is a frontend-only, stateless-on-the-backend change; the backend `internal/session/*` and `internal/handler/auth.go` are untouched and need no migration step.

## Codegen

Not applicable — no codegen tooling (sqlc/protobuf/openapi) in this repo, and this change touches no generated code.

## Risks

- **Single in-flight guard (`checking.current`) instead of per-request IDs.** Because `ensureChecked` refuses to start a new probe while one is already outstanding, there can never be two overlapping `GET /auth/me` calls from this hook, so the "stale probe resolves after a newer one" race described in the spec's edge case 1 can't actually occur — the second attempt is simply skipped, not raced. Plan B if testing (step 1e) reveals a gap (e.g. an effect re-running before `finally` clears the flag): fall back to the request-ID-per-call pattern described in the spec's Constraints section.
- **Treating any non-401 rejection as "keep authenticated" could mask a real problem** (e.g. a 403 from a future authorization layer that isn't session-related). Accepted for now — CLAUDE.md's hard rule requires a spec for new scope, and distinguishing 403-from-authz vs 401-from-session is not in `specs/session-revalidation/spec.md`; if that need arises it gets its own spec.
- **`status === "authenticated"` re-fires `setStatus("authenticated")` on every successful revalidation.** Harmless (React bails out re-rendering on an unchanged state value) but worth noting so it isn't mistaken for a bug during review.

## Out of scope guard

Do not touch: `internal/session/*`, `internal/handler/auth.go`, `internal/user/*`, `internal/otp/*`, `internal/job/*`, `internal/db/migrations/*`, `web/src/api/client.ts` (the `GET /auth/me` client function and 401 handling it already provides are reused unmodified), `web/src/features/*` (job screens are unaffected — they don't call `ensureChecked` themselves).

**Note:** `web/src/app/ProtectedRoute.tsx` is *not* in this do-not-touch list — the spec's Scope section names it explicitly (per CLAUDE.md's rule against a plan/spec contradiction by omission), even though step 1's investigation found its existing rendering logic needs no behavioral change, only a doc-comment update in step 2.
