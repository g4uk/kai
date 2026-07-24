import type { ReactNode } from "react";

export interface StatusMessageProps {
  children: ReactNode;
}

/**
 * Shared one-line loading/in-progress status presentation
 * (specs/ui-consistency/plan.md step 6, criterion 9). Takes children as
 * the message, no built-in text of its own.
 */
export function StatusMessage({ children }: StatusMessageProps) {
  return <p className="text-sm text-slate-600">{children}</p>;
}
