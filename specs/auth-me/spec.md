# Spec: Auth Me

## Problem

The `ui` spec's login screen must show for an unauthenticated visitor without making any request tied to job data (acceptance criterion 1: "no `/jobs` request is made"). No endpoint currently exists for the SPA to cheaply ask "is my session still valid?" without touching a resource-scoped endpoint — `specs/ui/plan.md` worked around this by overloading `GET /jobs` as an auth probe, which was flagged in that plan's Risks section as a literal deviation from criterion 1 and confirmed as such during verification. This spec adds the missing cheap check so the workaround can be removed.

## Scope

- `GET /auth/me` — behind the existing `SessionMiddleware` (from `specs/user-auth`), returns `204 No Content` with an empty body when the session cookie resolves to a valid session; returns `401` (via `SessionMiddleware`'s existing behavior) when it doesn't. No new dependencies, no new interfaces beyond what `SessionMiddleware` already provides — reaching the handler at all is sufficient proof of a valid session, so the handler itself does nothing but write `204`.

## Non-scope

- Returning any user data (`user_id`, phone number, etc.) in the response body — this is a boolean liveness check, not a profile endpoint; adding fields is deferred until a real consumer needs them (YAGNI).
- Any change to session creation/deletion, OTP flow, or `SessionMiddleware` itself.
- Removing the `ui` spec's `GET /jobs` auth-probe workaround — that's `specs/ui/plan.md`'s job to update as a follow-on to this spec landing, not this spec's.

## Acceptance criteria

1. When `GET /auth/me` is called with a valid session cookie, then the response is `204` with an empty body.
2. When `GET /auth/me` is called with no session cookie, or one that doesn't resolve to a live session, then the response is `401` (identical to `SessionMiddleware`'s existing behavior on any other protected route — no new logic).
3. When `go build ./cmd/...` is run, the `api` binary compiles with the new endpoint wired with zero errors.

## Constraints

- No new Go interfaces/dependencies: the handler needs zero collaborators (unlike `JobCreator`/`OTPRequester`/etc.), since `SessionMiddleware` having already run is the entire check.
- Security: identical to every other protected route — `SessionMiddleware`'s existing session-validation logic is reused unmodified, not reimplemented.
