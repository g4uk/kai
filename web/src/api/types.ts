// Hand-written to mirror the Go wire structs exactly (no OpenAPI/codegen
// exists in this repo — see specs/ui/plan.md's "Codegen" section).
// Source of truth: internal/handler/jobs.go.
//
// If internal/handler/jobs.go's JSON field names/shapes change, this file
// must be updated by hand in the same change.

/** Mirrors internal/handler/jobs.go's Job struct. */
export interface Job {
  id: number;
  youtube_url: string;
  status: string;
  created_at: string;
  updated_at: string;
}

/** Mirrors internal/handler/jobs.go's Metric struct. */
export interface Metric {
  key: string;
  value: number;
}

/** Mirrors internal/handler/jobs.go's Participant struct. */
export interface Participant {
  id: number;
  label: string;
  metrics: Metric[];
}

/** Mirrors internal/handler/jobs.go's JobDetail struct (Job + participants + summary). */
export interface JobDetail extends Job {
  participants: Participant[];
  summary: string | null;
}
