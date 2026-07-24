import { Link } from "react-router-dom";

export interface EmptyStateProps {
  heading: string;
  body: string;
  linkTo?: string;
  linkLabel?: string;
}

/**
 * Shared zero-content empty state (specs/ui-consistency/plan.md step 6),
 * used today only by JobListPage's zero-jobs state (criterion 10).
 */
export function EmptyState({
  heading,
  body,
  linkTo,
  linkLabel,
}: EmptyStateProps) {
  return (
    <div className="flex flex-col gap-2">
      <h2 className="text-lg font-semibold text-slate-900">{heading}</h2>
      <p className="text-sm text-slate-600">{body}</p>
      {linkTo !== undefined && linkLabel !== undefined && (
        <Link
          to={linkTo}
          className="text-sm text-blue-600 underline hover:text-blue-700"
        >
          {linkLabel}
        </Link>
      )}
    </div>
  );
}
