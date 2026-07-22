# Architecture Decisions

## 001: Go backend with Docker on Hetzner
Why: Go provides strong concurrency for async video analysis jobs; Docker ensures reproducible deploys; Hetzner offers cost-effective dedicated/VPS hardware for self-hosted workloads.
Rejected: Managed cloud (AWS/GCP) — unnecessary cost and complexity for a single-server deployment.
Cost: Team must manage server provisioning, TLS, and uptime manually.

## 002: MySQL as primary datastore, Redis as job queue and cache
Why: MySQL is battle-tested for relational data with foreign key integrity across users, jobs, participants, and metrics; Redis handles async job queueing and short-lived result caching without polling the DB.
Rejected: PostgreSQL — MySQL sufficient for this schema; NoSQL — relational structure is a better fit for the metric/participant model.
Cost: Two stateful services to back up and operate.

## 003: Data model — users, analysis_jobs, participants, participant_metrics, job_summaries
Why: Normalized schema separates concerns: a job belongs to a user, participants belong to a job, metrics are extensible key-value rows per participant, and summaries are stored separately for fast retrieval.
Rejected: Flat JSON blob per job — loses queryability and makes per-participant metric filtering hard.
Cost: More joins required; participant_metrics grows large at scale.

## 004: Docker Compose for service orchestration on a single Hetzner node
Why: Simple to operate for a single-server setup; all services (API, worker, MySQL, Redis) run as containers with a shared Compose network.
Rejected: Kubernetes — overkill for one node; bare metal — harder to reproduce and redeploy.
Cost: No horizontal scaling without migrating to an orchestrator.

## 005: User-scoped data isolation via user_id foreign keys
Why: Each resource (analysis_jobs, etc.) carries a user_id; all queries filter by the authenticated user's ID. Sufficient for a single-tenant-per-account model without schema-level separation.
Rejected: Schema-per-tenant — unnecessary complexity at current scale.
Cost: Relies on correct query-level filtering; no DB-enforced cross-user wall beyond FK constraints.

## 006: React SPA served via nginx reverse proxy in a new `web` Compose service
Why: First frontend framework choice for the project — gives the existing OTP auth and jobs API a user-facing surface (login, submit, track, view results). Serving the built React bundle through nginx on the same origin as the API (reverse-proxying `/api/*`) keeps the `SameSite=Strict` session cookie flowing on every request, with no cross-origin/shared-domain TLS workaround needed.
Rejected: A separate origin or subdomain for the SPA — would break the `SameSite=Strict` cookie set by the user-auth spec, or force relaxing that cookie attribute for no other reason than serving topology. Server-side rendering or a Go-templated UI — would couple UI release velocity to Go redeploys and duplicate the JSON API that's already built.
Cost: Two runtime images to build and deploy instead of one; the nginx proxy config is one more piece of infra that must stay in sync with the API's route surface if routes ever move or gain a prefix.
