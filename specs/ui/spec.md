# Spec: UI

## Problem

Coaches and athletes currently have no way to interact with the system except raw API calls — there is no screen to log in, submit a YouTube URL for analysis, check job status, or view the resulting per-participant metrics and match summaries. This spec delivers the end-to-end user-facing surface (login → submit → track → view results) as a React single-page app, so the OTP auth and data model already built have somewhere to be used.

## Scope

- React SPA covering four screens: login (phone + OTP), job list/dashboard, new-analysis submission form, and job results view
- Login screen: phone-number entry → `POST /auth/otp/request`, then OTP-code entry → `POST /auth/otp/verify`; on success, navigate to the job list
- Logout action: `POST /auth/logout`, then redirect to login and discard any client-cached job data
- Job list screen: `GET /jobs` — lists the authenticated user's own jobs (YouTube URL/title, status, created-at), with an empty-state for zero jobs
- New-analysis form: client-side YouTube URL format validation, then `POST /jobs`; surfaces server-side validation errors inline
- Results screen: `GET /jobs/:id` — renders one section per participant with every key/value pair from that participant's metrics (generic rendering, no hardcoded metric names), plus the job's overall summary; renders distinct states for `pending`/`processing`, `done`, and `failed` job statuses
- Global 401 handling: any API response of 401 clears client state and redirects to login
- New `web` Docker Compose service: builds the production React bundle and serves it via nginx, reverse-proxying `/api/*` to the existing `api` service so the SPA and API share one origin (required for the `SameSite=Strict` session cookie from the user-auth spec to be sent on API calls)

## Non-scope

- Implementing the `POST /jobs`, `GET /jobs`, and `GET /jobs/:id` backend handlers — this spec assumes that minimal REST contract exists or is delivered by a separate backend spec; this UI spec covers only the SPA and its `web` serving/proxy layer
- Real-time status updates: no WebSockets, Server-Sent Events, or automatic polling — job status reflects whatever the API returned at the time the page was loaded; the user must reload to see a status change
- Embedding or playing back the source YouTube video inline
- Exporting or sharing results (PDF, CSV, share links)
- Editing or deleting submitted jobs
- Pagination or infinite scroll on the job list (assumes a per-user job count small enough to render in one page for this MVP)
- Account/profile management screens (changing phone number, avatar, preferences)
- Roles, permissions, or any coach/team multi-athlete view — one user sees only their own jobs, per decision 005
- Internationalization/localization — English only
- Native mobile apps — responsive web only
- A local frontend dev server workflow (e.g. Vite HMR) as a Compose service — the `web` service builds the production bundle; local dev tooling choices are left to the implementer and out of scope for acceptance criteria

**Note:** This is the first frontend/SPA introduced into the project. `docs/decisions.md` currently records no frontend framework choice. Per CLAUDE.md, this deviation from prior architecture must be captured as a new decision entry (framework: React; serving: nginx + reverse proxy in a new `web` Compose service) at implementation time, not left implicit.

## Acceptance criteria

1. When an unauthenticated user loads the SPA root, then the login (phone-number) screen is shown and no `/jobs` request is made.
2. When a user submits a valid E.164 phone number on the login screen, then the SPA calls `POST /auth/otp/request` and transitions to the OTP-code entry screen.
3. When a user submits the correct OTP code within its TTL, then the SPA calls `POST /auth/otp/verify`, the returned session cookie is stored by the browser, and the SPA navigates to the job list screen.
4. When a user submits an incorrect or expired OTP code, then the SPA displays the API's error inline on the OTP screen and does not navigate away from it.
5. When the job list screen loads for an authenticated user with N existing jobs, then it calls `GET /jobs` and renders exactly N rows, each showing the YouTube URL (or title, if the API provides one), current status, and created-at timestamp.
6. When the job list screen loads for a user with zero jobs, then an empty-state message with a "submit your first analysis" call-to-action is shown instead of a table.
7. When a user submits a syntactically valid YouTube URL via the new-analysis form, then the SPA calls `POST /jobs` and, on a 2xx response, navigates to that job's results screen.
8. When a user submits a malformed URL (not a recognizable YouTube URL pattern) via the new-analysis form, then the SPA blocks submission client-side, shows an inline validation error, and does not call `POST /jobs`.
9. When `POST /jobs` returns a non-2xx response, then the SPA displays the server-provided error message inline on the form and does not navigate away.
10. When a user opens the results screen for a job with status `done`, then the SPA calls `GET /jobs/:id` and renders one section per participant containing every key/value pair present in that participant's metrics, plus the job's summary text.
11. When a participant in a `done` job has zero recorded metrics, then that participant's section renders an explicit "no metrics recorded" note rather than an empty or missing table.
12. When a user opens the results screen for a job with status `pending` or `processing`, then an in-progress state is shown and no metrics section is rendered.
13. When a user opens the results screen for a job with status `failed`, then a failure state is shown containing the API-provided error detail, and no metrics section is rendered.
14. When a user opens the results screen for a job ID belonging to another user, then the SPA renders a generic "not found" state from the backend's 404 — it never reveals whether the job ID exists for someone else.
15. When any API call from the SPA receives a 401 response, then the SPA clears client-held state and redirects to the login screen.
16. When a user clicks "Log out", then the SPA calls `POST /auth/logout`, redirects to login, and a subsequent browser back-navigation to the job list re-fetches from the API rather than rendering a cached authenticated view.
17. When `docker compose up --build` is run, the new `web` service starts, serves the production React bundle, and reverse-proxies `/api/*` requests to the `api` service on the same origin the page was loaded from.
18. When the SPA calls an `/api/*` endpoint after a successful login, then the session cookie set by `/auth/otp/verify` is sent automatically with that request (confirming the reverse proxy preserves same-origin cookie behavior under `SameSite=Strict`).

## Edge cases

1. **Foreign job ID lookup** — a user opens `/jobs/:id` for a job owned by a different `user_id`; the backend's 404 is shown as a generic not-found state, never distinguishing "doesn't exist" from "not yours" (criterion 14).
2. **Session expires mid-session** — any in-flight API call returns 401 after the cookie has expired or been invalidated server-side; the SPA redirects to login and drops any unsaved form input (e.g., a partially typed YouTube URL) rather than silently retrying with stale credentials (criterion 15).
3. **Job stuck in `processing`** — the results screen for a non-terminal job never renders a partial or broken metrics table; it shows the in-progress state until the user reloads and the API reports `done` or `failed` (criterion 12).
4. **Job `failed` with no error detail from the API** — if the API's error field is empty/null, the failure state still renders without crashing (shows a generic fallback message rather than "undefined" or a blank string).
5. **Participant with zero metrics** — a `done` job where one participant has an empty metrics object still renders that participant's name/section with an explicit empty note, not a blank gap in the layout (criterion 11).
6. **New account, zero jobs** — job list renders the empty-state, not a loading spinner stuck indefinitely or a raw empty array artifact (criterion 6).
7. **OTP resubmission during login** — if a user re-submits the OTP form after a prior invalid attempt, the SPA does not double-submit or leave the form in a stuck "submitting" state on repeated failures (client-side only; server-side attempt limiting is out of scope, covered by the user-auth spec).
8. **Deep link to a job while logged out** — a user follows a direct link to `/jobs/:id` without a valid session; the SPA redirects to login rather than issuing the API call unauthenticated (consistent with criterion 1's root-level behavior).

## Constraints

- Security: the SPA never stores the session token in `localStorage`/`sessionStorage`; it relies exclusively on the `HttpOnly`, `Secure`, `SameSite=Strict` cookie issued by the user-auth spec, meaning the `web` service's reverse proxy to `api` must keep both under the same origin — cross-origin calls would silently drop the cookie and appear as 401s.
- Security: every job/results request is scoped server-side by `user_id` (per CLAUDE.md's hard rule); the UI must treat any 404 on `/jobs/:id` as authoritative and never attempt to infer or display data for a job it wasn't given by the API.
- Compatibility: supports the latest two stable releases of Chrome, Firefox, and Safari; no legacy/IE support required.
- Compatibility: no ORM/backend changes — this spec is presentation-only and introduces no new Go code, migrations, or business logic.
- Performance: job list and results screens render usable content within 2 seconds on a 10 Mbps connection for job counts up to 50 (no pagination is implemented at this scope, so this is the practical ceiling for MVP usage).
- Deployment: the new `web` Compose service adds a `healthcheck` stanza consistent with the existing `mysql`/`redis` services, so `docker compose up` reports it healthy before being considered part of a working stack.
- Architecture: introduces the project's first frontend framework decision (React + nginx reverse proxy); per CLAUDE.md, this must be recorded as a new numbered entry in `docs/decisions.md` at implementation time, not left as an undocumented default.
