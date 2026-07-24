import type { ReactNode } from "react";

export interface AlertProps {
  children?: ReactNode;
}

/** Shared role="alert" display for validation/server errors (specs/ui-consistency/plan.md step 3). */
export function Alert({ children }: AlertProps) {
  if (children === null || children === undefined) {
    return null;
  }

  return (
    <p
      role="alert"
      className="rounded-md border border-red-300 px-3 py-2 text-sm text-red-600"
    >
      {children}
    </p>
  );
}
