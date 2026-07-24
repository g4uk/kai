import type { ReactNode } from "react";

export interface PageHeaderProps {
  title: string;
  children?: ReactNode;
}

/**
 * Shared page-title + action-slot header (specs/ui-consistency/plan.md
 * step 5). The `<h1>` classes here are the one shared page-title
 * combination used across all five screens (criterion 11).
 */
export function PageHeader({ title, children }: PageHeaderProps) {
  return (
    <div className="flex items-center justify-between">
      <h1 className="text-xl font-semibold text-slate-900">{title}</h1>
      {children}
    </div>
  );
}
