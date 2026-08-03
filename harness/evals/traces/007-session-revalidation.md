# 007: session-revalidation — re-check session validity on every protected-route navigation

## Prompt

Add session revalidation to the existing React SPA's protected routes
(`web/src/app/AuthContext.tsx`, `web/src/app/ProtectedRoute.tsx`): today
`GET /auth/me` is only probed once, on the very first protected-route mount
while status is `"unknown"`; once authenticated, navigating between `/jobs`,
`/jobs/new`, and `/jobs/:id` never re-checks whether the session is still
valid, so an expired or server-invalidated session renders as if still
logged in until some unrelated API call happens to 401. Change
`ProtectedRoute` to re-probe `GET /auth/me` on every navigation into a
protected route (not just the first mount) — since React Router reuses the
same `ProtectedRoute` instance across sibling route matches instead of
remounting it, this requires keying the revalidation effect off
`useLocation().pathname`, not component mount alone. While already
`"authenticated"`, render the destination optimistically without waiting for
the new probe (no blocking spinner); only demote to `"anonymous"` and
redirect to `/login` if the probe comes back with an actual `401` — a
non-401 failure (network hiccup, 5xx) during revalidation must leave an
already-authenticated session alone. Guard against overlapping probes so
rapid navigation never has two `GET /auth/me` calls in flight at once. The
first-load `"unknown"` → block-until-resolved behavior stays unchanged.

## Checks

- [ ] cmd: cd web && npm ci && OUT=$(npx vitest run --no-color src/app/router.test.tsx -t "re-probes getAuthMe on every navigation into a protected route while authenticated" 2>&1); echo "$OUT"; echo "$OUT" | grep -qE '[1-9][0-9]* passed'
- [ ] cmd: cd web && npm ci && OUT=$(npx vitest run --no-color src/app/router.test.tsx -t "redirects to /login once a pending revalidation probe rejects with a 401" 2>&1); echo "$OUT"; echo "$OUT" | grep -qE '[1-9][0-9]* passed'
- [ ] cmd: cd web && npm ci && OUT=$(npx vitest run --no-color src/app/router.test.tsx -t "does not redirect to /login when the revalidation probe fails" 2>&1); echo "$OUT"; echo "$OUT" | grep -qE '\b2 passed'
- [ ] cmd: cd web && npm ci && npm test -- --run
