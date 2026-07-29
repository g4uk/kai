import { useEffect } from "react";
import { useAuth } from "../../app/AuthContext";
import { useToast } from "../../ui/Toast";

interface JobStatusPayload {
  job_id: number;
  status: string;
}

/** Terminal statuses get distinct wording from an in-progress `processing` status (spec criterion 4). */
function toastMessageFor(payload: JobStatusPayload): string {
  if (payload.status === "processing") {
    return `Job #${payload.job_id} is now processing`;
  }
  return `Job #${payload.job_id} finished: ${payload.status}`;
}

/**
 * Side-effect-only hook (specs/popup-notifications+sse/plan.md step 7):
 * while authenticated, opens a credentialed EventSource to /api/jobs/stream
 * and forwards each `job_status` message to useToast().addToast(...). The
 * effect re-runs — closing any previous connection — whenever auth status
 * changes, which is what makes logout close the stream (spec criterion 6).
 */
export function useJobStatusEvents(): void {
  const { status, ensureChecked } = useAuth();
  const { addToast } = useToast();

  // Mounted at the app root (App.tsx) alongside, not necessarily beneath,
  // ProtectedRoute — on a route ProtectedRoute doesn't guard yet (e.g. first
  // paint before navigation settles), nothing else has triggered the
  // initial GET /auth/me probe. Trigger it here too so `status` reliably
  // leaves "unknown" without depending on ProtectedRoute having already
  // mounted; ensureChecked is idempotent/self-guarding (AuthContext.tsx).
  useEffect(() => {
    ensureChecked();
  }, [ensureChecked]);

  useEffect(() => {
    if (status !== "authenticated") {
      return;
    }

    const source = new EventSource("/api/jobs/stream");

    const handleJobStatus = (event: MessageEvent<string>) => {
      const payload = JSON.parse(event.data) as JobStatusPayload;
      addToast(toastMessageFor(payload));
    };

    source.addEventListener("job_status", handleJobStatus);

    return () => {
      source.removeEventListener("job_status", handleJobStatus);
      source.close();
    };
  }, [status, addToast]);
}
