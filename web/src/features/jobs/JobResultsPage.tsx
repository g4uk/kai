import { useParams } from "react-router-dom";
import { ApiError } from "../../api/client";
import { useJob } from "./useJob";
import type { Participant } from "../../api/types";
import { LogoutButton } from "../../app/AuthContext";
import { PageHeader } from "../../ui/PageHeader";
import { StatusMessage } from "../../ui/StatusMessage";

function ParticipantSection({ participant }: { participant: Participant }) {
  return (
    <section aria-label={participant.label} className="flex flex-col gap-2">
      <h2 className="text-lg font-semibold text-slate-900">
        {participant.label}
      </h2>
      {participant.metrics.length === 0 ? (
        <p>No metrics recorded.</p>
      ) : (
        <dl className="flex flex-col gap-1">
          {participant.metrics.map((metric) => (
            <div
              key={metric.key}
              className="flex min-w-0 flex-wrap gap-2 text-sm"
            >
              <dt className="min-w-0 break-words font-medium text-slate-600">
                {metric.key}
              </dt>
              <dd className="min-w-0 break-words text-slate-900">
                {metric.value}
              </dd>
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
    return (
      <div className="flex flex-col gap-4 p-4">
        <PageHeader title="Results">
          <LogoutButton />
        </PageHeader>
        <StatusMessage>Loading…</StatusMessage>
      </div>
    );
  }

  if (error instanceof ApiError && error.status === 404) {
    return (
      <div className="flex flex-col gap-4 p-4">
        <PageHeader title="Results">
          <LogoutButton />
        </PageHeader>
        <p>Job not found.</p>
      </div>
    );
  }

  if (error !== null) {
    return (
      <div className="flex flex-col gap-4 p-4">
        <PageHeader title="Results">
          <LogoutButton />
        </PageHeader>
        <p>Something went wrong loading this job.</p>
      </div>
    );
  }

  if (job === null) {
    return null;
  }

  if (job.status === "pending" || job.status === "processing") {
    return (
      <div className="flex flex-col gap-4 p-4">
        <PageHeader title="Results">
          <LogoutButton />
        </PageHeader>
        <StatusMessage>Analysis in progress…</StatusMessage>
      </div>
    );
  }

  if (job.status === "failed") {
    return (
      <div className="flex flex-col gap-4 p-4">
        <PageHeader title="Results">
          <LogoutButton />
        </PageHeader>
        <p>
          {job.summary && job.summary.length > 0
            ? job.summary
            : "Something went wrong analyzing this job."}
        </p>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-6 p-4">
      <PageHeader title="Results">
        <LogoutButton />
      </PageHeader>
      {job.summary !== null && job.summary.length > 0 && <p>{job.summary}</p>}
      {job.participants.map((participant) => (
        <ParticipantSection key={participant.id} participant={participant} />
      ))}
    </div>
  );
}
