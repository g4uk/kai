import type { HTMLAttributes, TdHTMLAttributes, ThHTMLAttributes } from "react";

export type TableProps = HTMLAttributes<HTMLTableElement>;
export type TableSectionProps = HTMLAttributes<HTMLTableSectionElement>;
export type TableRowProps = HTMLAttributes<HTMLTableRowElement>;
export type TableHeaderCellProps = ThHTMLAttributes<HTMLTableCellElement>;
export type TableCellProps = TdHTMLAttributes<HTMLTableCellElement>;

/**
 * Shared table primitives (specs/ui-consistency/plan.md step 4). The
 * `overflow-x-auto` wrapper lets a wide table scroll within its own
 * container instead of the page overflowing horizontally (criterion 13).
 */
export function Table({ className, ...rest }: TableProps) {
  const classes = className
    ? `min-w-full divide-y divide-slate-200 ${className}`
    : "min-w-full divide-y divide-slate-200";
  return (
    <div className="overflow-x-auto">
      <table className={classes} {...rest} />
    </div>
  );
}

export function TableHead({ className, ...rest }: TableSectionProps) {
  const classes = className ? `bg-slate-50 ${className}` : "bg-slate-50";
  return <thead className={classes} {...rest} />;
}

export function TableBody({ className, ...rest }: TableSectionProps) {
  const classes = className
    ? `divide-y divide-slate-200 ${className}`
    : "divide-y divide-slate-200";
  return <tbody className={classes} {...rest} />;
}

export function TableRow({ ...rest }: TableRowProps) {
  return <tr {...rest} />;
}

export function TableHeaderCell({ className, ...rest }: TableHeaderCellProps) {
  const classes = className
    ? `px-4 py-2 text-left text-sm font-semibold text-slate-700 ${className}`
    : "px-4 py-2 text-left text-sm font-semibold text-slate-700";
  return <th className={classes} {...rest} />;
}

export function TableCell({ className, ...rest }: TableCellProps) {
  const classes = className
    ? `px-4 py-2 text-sm text-slate-700 ${className}`
    : "px-4 py-2 text-sm text-slate-700";
  return <td className={classes} {...rest} />;
}
