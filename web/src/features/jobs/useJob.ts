import { useEffect, useState } from "react";
import { getJob } from "../../api/client";
import type { JobDetail } from "../../api/types";

export interface UseJobResult {
  job: JobDetail | null;
  loading: boolean;
  error: unknown | null;
}

/**
 * Wraps getJob(id) with loading/error/data state (specs/ui/plan.md step 7).
 * `error` is kept as the raw thrown value (not stringified) so the page can
 * branch on `error instanceof ApiError && error.status === 404` (criterion 14).
 */
export function useJob(id: number): UseJobResult {
  const [job, setJob] = useState<JobDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown | null>(null);

  useEffect(() => {
    let cancelled = false;

    getJob(id)
      .then((result) => {
        if (cancelled) return;
        setJob(result);
        setError(null);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setError(err);
        setJob(null);
      })
      .finally(() => {
        if (cancelled) return;
        setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [id]);

  return { job, loading, error };
}
