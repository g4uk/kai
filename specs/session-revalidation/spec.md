# Spec: Session Revalidation

## Problem

`AuthContext` (`web/src/app/AuthContext.tsx`) probes `GET /auth/me` exactly once per app load, the first time `ProtectedRoute` mounts while `status === "unknown"`. Once `status` becomes `"authenticated"` it is never re-checked on subsequent navigations — so a user can navigate to `/jobs`, `/jobs/new`, or `/jobs/:id` after their session has expired or been invalidated server-side (e.g. after a logout elsewhere, or TTL expiry) and the page renders as if still logged in. Today the only thing that catches this is an incidental 401 from a page's own data call (e.g. `listJobs`), which may lag behind the render or never fire on pages that don't immediately call the API. This spec makes session validity re-checked on every protected-route mount, closing that gap.

## Scope

- `ProtectedRoute` (`web/src/app/ProtectedRoute.tsx`) triggers a `GET /auth/me` probe every time it mounts (i.e. on every navigation into `/jobs`, `/jobs/new`, or `/jobs/:id`), not only when `status === "unknown"`.
- When the prior status is already `"authenticated"`, the mount-time probe runs in the background and protected content renders immediately from the existing status (optimistic) — no blocking spinner or blank screen is introduced for the already-authenticated case.
- If the background probe resolves 401 (or otherwise fails), `AuthContext` transitions `status` to `"anonymous"`, which causes `ProtectedRoute` to redirect to `/login` — reusing the same status/redirect mechanism `ProtectedRoute` already has for `"anonymous"`.
- If the background probe succeeds (204), there is no visible change; `status` stays `"authenticated"`.
- The existing first-load behavior (`status === "unknown"`: probe once, render nothing until resolved) is unchanged.
- The existing reactive 401 handling in `web/src/api/client.ts` (`onUnauthorized` → `status = "anonymous"`) is unchanged and continues to run independently of the mount-time probe.

## Non-scope

- Periodic/interval polling of `GET /auth/me` while a protected route stays mounted without a navigation — this spec is mount-triggered only; polling is deferred until a concrete need for it exists (YAGNI).
- Revalidating on browser tab refocus/visibility change — explicitly not part of this spec's trigger.
- Any change to session TTL, `SessionMiddleware`, `Store.Validate`, or other backend session semantics (`internal/session/session.go`, `internal/handler/auth.go`) — reuses the existing `GET /auth/me` endpoint unmodified.
- Blocking/gating rendering of protected content while the mount-time probe is in flight for an already-`"authenticated"` status — this spec is explicitly optimistic-render, not a loading-gate redesign.
- Any change to `POST /auth/logout`, `POST /auth/otp/*`, or the login/OTP flow.
- De-duplicating or debouncing rapid repeated navigations beyond what's needed to avoid overlapping in-flight probes (see Constraints) — no new UI affordance (e.g. a toast) is added for this.

## Acceptance criteria

1. When a user navigates to `/jobs`, `/jobs/new`, or `/jobs/:id` while `status` is already `"authenticated"`, then the SPA calls `GET /auth/me` again for that navigation (in addition to the one-time first-load probe).
2. When that mount-time probe returns `204`, then no visible change occurs — the protected page is already rendered and stays rendered, with no flash, spinner, or re-render of its content caused by the probe.
3. When that mount-time probe returns `401` (session expired or invalidated server-side since the last check), then `status` transitions to `"anonymous"` and the SPA redirects to `/login`, the same as the existing reactive-401 redirect path.
4. When a user navigates from one protected route to another (e.g. `/jobs` → `/jobs/123`) while still authenticated, then the destination route's content renders immediately without waiting for the new probe to resolve (optimistic render), per Scope.
5. When the app first loads with `status === "unknown"`, then behavior is unchanged from today: the probe blocks rendering (renders nothing) until it resolves, then shows content or redirects accordingly.
6. When a user is on a protected route and some other in-page API call (e.g. `listJobs`) independently returns 401 before the mount-time probe resolves, then the existing reactive handling in `web/src/api/client.ts` still redirects to `/login` — the two mechanisms don't conflict or double-redirect in a broken way.

## Edge cases

1. **Rapid navigation between protected routes** — a user clicks `/jobs` → `/jobs/123` → `/jobs` in quick succession; each mount fires its own probe, but an earlier in-flight probe resolving after a later one must not incorrectly flip `status` back to `"authenticated"` after a real 401 was already observed (last-resolved-wins is not acceptable if it clobbers a redirect already in progress).
2. **Probe network failure (not 401)** — the `GET /auth/me` fetch itself fails (offline, timeout, 5xx) during a mount-time revalidation; the SPA does not treat a non-401 failure the same as a 401 (it must not force-logout a user whose session is actually fine but whose network hiccuped). Behavior mirrors `ApiError` handling already in `client.ts` (only `status === 401` triggers `onUnauthorized`).
3. **Session invalidated while filling out the new-analysis form** — a user is on `/jobs/new` with a partially typed YouTube URL when their session expires server-side; the mount-time probe already ran when the page loaded, so this edge case is only caught if the user re-navigates or a data call 401s (consistent with Non-scope's exclusion of polling/refocus revalidation — not fully closed by this spec, and should not be implied as fixed by it).
4. **Logout followed by browser back-navigation** — after `LogoutButton` clears state and navigates to `/login` (`web/src/app/AuthContext.tsx`'s `logout()`), the user hits browser "back" to `/jobs`; `ProtectedRoute` remounts, fires a fresh probe, gets 401 (session was deleted server-side by `POST /auth/logout`), and redirects — this must keep working exactly as `specs/ui/spec.md` criterion 16 already requires.
5. **First protected-route mount of the app** — must still be governed by the existing `status === "unknown"` first-load path (criterion 5), not accidentally double-probed by both the "unknown" logic and the new mount-time revalidation logic firing separately for the same mount.
6. **Deep link directly to `/jobs/:id` while already logged in elsewhere in another tab, but this tab's session cookie is stale/cleared** — mount-time probe on first load still follows the `"unknown"` path (edge case 5), not the revalidation path, since there is no prior `"authenticated"` status to revalidate from.

## Constraints

- Performance: the mount-time probe must not block or delay rendering of already-`"authenticated"` protected content (Scope) — it is fire-and-forget from the render's perspective, only acting on failure.
- Concurrency: overlapping probes (e.g. from rapid navigation) must not race such that a stale probe's resolution overwrites a newer probe's result or an in-progress redirect (edge case 1) — reuse or extend the existing `checking` ref guard pattern in `AuthContext.tsx`, adapted so it no longer skips probing when `status !== "unknown"`.
- Security: no new client-side storage of session state beyond the existing in-memory `status` — still no `localStorage`/`sessionStorage` use, consistent with `specs/ui/spec.md`'s cookie-only constraint.
- Compatibility: no backend changes; reuses `GET /auth/me` from `specs/auth-me/spec.md` unmodified.
- No new Go code, migrations, or API endpoints — this is a frontend-only change confined to `web/src/app/AuthContext.tsx` and `web/src/app/ProtectedRoute.tsx`.
