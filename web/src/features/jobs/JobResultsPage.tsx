import { useParams } from "react-router-dom";
import { ApiError } from "../../api/client";
import { useJob } from "./useJob";
import type { Participant } from "../../api/types";

function ParticipantSection({ participant }: { participant: Participant }) {
  return (
    <section aria-label={participant.label} className="flex flex-col gap-2">
      <h2 className="font-semibold">{participant.label}</h2>
      {participant.metrics.length === 0 ? (
        <p>No metrics recorded.</p>
      ) : (
        <dl>
          {participant.metrics.map((metric) => (
            <div key={metric.key} className="flex gap-2">
              <dt>{metric.key}</dt>
              <dd>{metric.value}</dd>
            </div>
          ))}
        </dl>
      )}
    </section>
  );
}

/** Results screen — branches purely on job status (specs/ui/plan.md step 7). */
export function JobResultsPage() {
  const { id } = useParams<{ id: string }>();
  const { job, loading, error } = useJob(Number(id));

  if (loading) {
    return <p>Loading…</p>;
  }

  if (error instanceof ApiError && error.status === 404) {
    return <p>Job not found.</p>;
  }

  if (error !== null) {
    return <p>Something went wrong loading this job.</p>;
  }

  if (job === null) {
    return null;
  }

  if (job.status === "pending" || job.status === "processing") {
    return <p>Analysis in progress…</p>;
  }

  if (job.status === "failed") {
    return (
      <p>
        {job.summary && job.summary.length > 0
          ? job.summary
          : "Something went wrong analyzing this job."}
      </p>
    );
  }

  return (
    <div className="flex flex-col gap-6 p-4">
      <h1 className="text-xl font-semibold">Results</h1>
      {job.summary !== null && job.summary.length > 0 && <p>{job.summary}</p>}
      {job.participants.map((participant) => (
        <ParticipantSection key={participant.id} participant={participant} />
      ))}
    </div>
  );
}
