import { Link } from "react-router-dom";
import { useJobs } from "./useJobs";
import { LogoutButton } from "../../app/AuthContext";

/** Job list / dashboard screen (specs/ui/plan.md step 5). */
export function JobListPage() {
  const { jobs, loading, error } = useJobs();

  return (
    <div className="flex flex-col gap-4 p-4">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">Jobs</h1>
        <LogoutButton />
      </div>

      {loading && <p>Loading…</p>}
      {error !== null && <p role="alert">{error}</p>}

      {!loading && error === null && jobs.length === 0 && (
        <div className="flex flex-col gap-2">
          <p>You have not submitted any analyses yet.</p>
          <Link to="/jobs/new" className="underline">
            Submit your first analysis
          </Link>
        </div>
      )}

      {!loading && jobs.length > 0 && (
        <table>
          <thead>
            <tr>
              <th>YouTube URL</th>
              <th>Status</th>
              <th>Created at</th>
            </tr>
          </thead>
          <tbody>
            {jobs.map((job) => (
              <tr key={job.id}>
                <td>
                  <Link to={`/jobs/${job.id}`}>{job.youtube_url}</Link>
                </td>
                <td>{job.status}</td>
                <td>{job.created_at}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
