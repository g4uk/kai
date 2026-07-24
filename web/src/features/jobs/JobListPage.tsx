import { Link } from "react-router-dom";
import { useJobs } from "./useJobs";
import { LogoutButton } from "../../app/AuthContext";
import { PageHeader } from "../../ui/PageHeader";
import { EmptyState } from "../../ui/EmptyState";
import { StatusMessage } from "../../ui/StatusMessage";
import { Alert } from "../../ui/Alert";
import {
  Table,
  TableHead,
  TableBody,
  TableRow,
  TableHeaderCell,
  TableCell,
} from "../../ui/Table";

/** Job list / dashboard screen (specs/ui/plan.md step 5). */
export function JobListPage() {
  const { jobs, loading, error } = useJobs();

  return (
    <div className="flex flex-col gap-4 p-4">
      <PageHeader title="Jobs">
        <LogoutButton />
      </PageHeader>

      {loading && <StatusMessage>Loading…</StatusMessage>}
      <Alert>{error}</Alert>

      {!loading && error === null && jobs.length === 0 && (
        <EmptyState
          heading="No analyses yet"
          body="You have not submitted any analyses yet."
          linkTo="/jobs/new"
          linkLabel="Submit your first analysis"
        />
      )}

      {!loading && jobs.length > 0 && (
        <Table>
          <TableHead>
            <TableRow>
              <TableHeaderCell>YouTube URL</TableHeaderCell>
              <TableHeaderCell>Status</TableHeaderCell>
              <TableHeaderCell>Created at</TableHeaderCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {jobs.map((job) => (
              <TableRow key={job.id}>
                <TableCell>
                  <Link
                    to={`/jobs/${job.id}`}
                    className="text-blue-600 underline hover:text-blue-700"
                  >
                    {job.youtube_url}
                  </Link>
                </TableCell>
                <TableCell>{job.status}</TableCell>
                <TableCell>{job.created_at}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  );
}
