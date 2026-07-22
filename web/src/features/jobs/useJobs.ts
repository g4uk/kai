import { useEffect, useState } from "react";
import { listJobs } from "../../api/client";
import type { Job } from "../../api/types";

export interface UseJobsResult {
  jobs: Job[];
  loading: boolean;
  error: string | null;
}

function messageOf(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return String(err);
}

/** Wraps listJobs() with loading/error/data state (specs/ui/plan.md step 5). */
export function useJobs(): UseJobsResult {
  const [jobs, setJobs] = useState<Job[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    listJobs()
      .then((result) => {
        if (cancelled) return;
        setJobs(result);
        setError(null);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setError(messageOf(err));
        setJobs([]);
      })
      .finally(() => {
        if (cancelled) return;
        setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, []);

  return { jobs, loading, error };
}
