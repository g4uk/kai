# Plan: Session Revalidation

Source: `specs/session-revalidation/spec.md`

Conventions confirmed from the codebase: frontend-only change, same conventions as `specs/auth-me/plan.md` step 3 and `specs/ui/plan.md` step 4 — Vitest + Testing Library, tests live in `web/src/app/router.test.tsx` (the existing home for all `AuthContext`/`ProtectedRoute`/routing behavior, per that file's own header comment), `../api/client` mocked via `vi.mock`. No new files needed — this is a small, surgical change to one hook.

**AMENDMENT (post-implementation-attempt correction):** the original investigation finding below turned out to be wrong, discovered when the implementer instrumented `ProtectedRoute` and found React Router **reuses the same `ProtectedRoute` component instance** across sibling `<Route>` matches at the same tree position (e.g. `/jobs` → `/jobs/123`) — it re-renders, it does not unmount/remount. Since `status` doesn't change across that navigation, `ensureChecked`'s reference can't change either, so its `useEffect` (deps: `[ensureChecked]`) never re-fires on sibling-route navigation. The original "no `ProtectedRoute.tsx` code changes" conclusion is retracted; see the corrected Step 1 below. The stale reasoning is left in place (not deleted) so the correction is traceable.

~~Investigation finding (drives the whole plan): `ProtectedRoute`'s `useEffect(() => { ensureChecked(); }, [ensureChecked])` in `web/src/app/ProtectedRoute.tsx` **already fires on every mount** — each protected `<Route>` in `router.tsx` wraps its own `<ProtectedRoute>` instance, so navigating `/jobs` → `/jobs/123` unmounts/remounts it. The only thing suppressing revalidation today is `ensureChecked`'s internal guard in `AuthContext.tsx`: `if (status !== "unknown" || checking.current) return;`. **`ProtectedRoute.tsx` itself needs no code changes** — its existing `status === "unknown"` (block) / `"anonymous"` (redirect) / else (render) branching already produces the exact optimistic behavior the spec asks for, once `AuthContext` actually re-probes. This is called out explicitly (not silently omitted) because `specs/session-revalidation/spec.md`'s Scope section names `ProtectedRoute.tsx` directly.~~

**Corrected finding:** `ProtectedRoute.tsx` DOES need a code change: its effect must depend on something that changes per-navigation even without a remount. Fix: add `useLocation().pathname` to the effect's dependency array, so `ensureChecked()` is called again on every path change regardless of whether the component instance persisted. This is the smallest change that produces the mount-or-navigate-triggered revalidation the spec requires, and was confirmed by the user over a `key`-prop-forced-remount alternative (blunter — full remount vs. re-running one effect).

Separately, implementing the `AuthContext.tsx` guard change surfaced a second, independent bug: with `ensureChecked` keyed on `[status]`, the `"unknown" → "authenticated"` transition from the *first* probe recreates `ensureChecked`'s reference, which — once `ProtectedRoute` also depends on `pathname` — could still re-trigger the effect an extra time within the *same* mount (no navigation involved), firing a spurious duplicate probe. Fix: mirror `status` into a ref (`statusRef`) inside `AuthContext.tsx` so `ensureChecked` has a stable, empty-deps identity that reads current status via the ref instead of closing over the `status` value directly. This keeps `ensureChecked`'s reference stable across `status` transitions within one mount, while `ProtectedRoute`'s new `pathname` dependency is what actually drives re-probing on navigation.

## Steps

1. **`AuthContext.tsx` + `ProtectedRoute.tsx` — re-probe on every navigation, with 401-only demotion on revalidation**
   - Files: `web/src/app/AuthContext.tsx`, `web/src/app/ProtectedRoute.tsx`, `web/src/app/router.test.tsx`
   - What, in `AuthContext.tsx`:
     - Import `ApiError` from `../api/client` (already exported, used elsewhere via `client.ts`'s own tests).
     - Change `ensureChecked`'s guard from `if (status !== "unknown" || checking.current) return;` to `if (status === "anonymous" || checking.current) return;` — so it still runs once for `"unknown"` (first load, unchanged) **and now also** for `"authenticated"` (revalidation, whenever `ProtectedRoute` calls it again), but still skips `"anonymous"` (already redirecting — a probe there is wasted work) and skips while a probe is already in flight (single global in-flight guard — see Risks for why this is sufficient to prevent races without extra request-ID bookkeeping).
     - Mirror `status` into a ref (`statusRef`, kept in sync via a `useEffect`) and give `ensureChecked` a stable, empty-deps identity that reads `statusRef.current` instead of closing over `status` directly (see the corrected finding above — needed so an `"unknown" → "authenticated"` transition mid-mount doesn't itself recreate `ensureChecked` and cause a spurious duplicate probe once `ProtectedRoute` also re-invokes it on every render whose `pathname` changed).
     - Capture `const wasAuthenticated = statusRef.current === "authenticated";` before firing the probe.
     - On success: `setStatus("authenticated")` (unchanged — a no-op re-render when already authenticated).
     - On failure: if `wasAuthenticated` and the error is *not* a `401` `ApiError` (i.e. `!(err instanceof ApiError && err.status === 401)`), do nothing — leave `status` as `"authenticated"` (spec edge case 2: a network hiccup or 5xx during revalidation must not force a logout). Otherwise (first-load `"unknown"` path, unchanged; or revalidation that got a real `401`), `setStatus("anonymous")`.
   - What, in `ProtectedRoute.tsx`:
     - Import `useLocation` from `react-router-dom` and add `const { pathname } = useLocation();` plus `pathname` to the existing effect's dependency array: `useEffect(() => { ensureChecked(); }, [ensureChecked, pathname]);` — this is the piece that actually triggers re-probing on sibling-route navigation (`/jobs` → `/jobs/123`), since React Router reuses the component instance there instead of remounting it (see corrected finding above). Rendering logic (`"unknown"`/`"anonymous"`/else branches) is unchanged.
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
   - What: re-run the existing first-load and deep-link-while-logged-out tests (`router.test.tsx`'s criterion-1 and edge-case-8 tests) to confirm the `"unknown"` path's blocking/redirect behavior is byte-for-byte unchanged. Update the header comments on `AuthContext.tsx` (currently describes only the mount-time probe as one-shot) and `ProtectedRoute.tsx` (currently says "triggers a probe" without noting it now re-fires on every `pathname` change, not just on mount) to reference `specs/session-revalidation/spec.md`, so the doc comments don't keep describing the pre-revalidation behavior as current.
   - No production logic changes in this step — comments and verification only.
   - Verify: `cd web && npm test -- --run` (full suite, confirming zero regressions across `JobListPage.test.tsx`, `NewJobPage.test.tsx`, `JobResultsPage.test.tsx`, `router.test.tsx`, `client.test.ts`).

## Order

Step 1 (core logic + its own tests, one commit) before step 2 (doc comments + full-suite re-verification, no logic change). No DB migration — this is a frontend-only, stateless-on-the-backend change; the backend `internal/session/*` and `internal/handler/auth.go` are untouched and need no migration step.

## Codegen

Not applicable — no codegen tooling (sqlc/protobuf/openapi) in this repo, and this change touches no generated code.

## Risks

- **React Router's instance-reuse across sibling routes (realized, not hypothetical).** The original plan assumed sibling `<Route>` matches remount `ProtectedRoute`; they don't (confirmed by instrumentation during implementation). This is now fixed via `pathname` in `ProtectedRoute`'s effect deps (Step 1), but it's the kind of framework-reconciliation assumption worth double-checking with a real render trace rather than reasoning from the JSX shape alone, on any future change to this component tree.
- **Single in-flight guard (`checking.current`) instead of per-request IDs.** Because `ensureChecked` refuses to start a new probe while one is already outstanding, there can never be two overlapping `GET /auth/me` calls from this hook, so the "stale probe resolves after a newer one" race described in the spec's edge case 1 can't actually occur — the second attempt is simply skipped, not raced. Plan B if testing (step 1e) reveals a gap (e.g. an effect re-running before `finally` clears the flag): fall back to the request-ID-per-call pattern described in the spec's Constraints section.
- **Treating any non-401 rejection as "keep authenticated" could mask a real problem** (e.g. a 403 from a future authorization layer that isn't session-related). Accepted for now — CLAUDE.md's hard rule requires a spec for new scope, and distinguishing 403-from-authz vs 401-from-session is not in `specs/session-revalidation/spec.md`; if that need arises it gets its own spec.
- **`status === "authenticated"` re-fires `setStatus("authenticated")` on every successful revalidation.** Harmless (React bails out re-rendering on an unchanged state value) but worth noting so it isn't mistaken for a bug during review.

## Out of scope guard

Do not touch: `internal/session/*`, `internal/handler/auth.go`, `internal/user/*`, `internal/otp/*`, `internal/job/*`, `internal/db/migrations/*`, `web/src/api/client.ts` (the `GET /auth/me` client function and 401 handling it already provides are reused unmodified), `web/src/features/*` (job screens are unaffected — they don't call `ensureChecked` themselves).

**Note:** `web/src/app/ProtectedRoute.tsx` is *not* in this do-not-touch list — the spec's Scope section names it explicitly (per CLAUDE.md's rule against a plan/spec contradiction by omission). Per the amendment above, it does end up needing a real (small) behavioral change in step 1 (the `useLocation().pathname` effect dependency), not just a doc-comment update — the original plan's claim that it needed no change was itself the thing that turned out to be wrong.
