import { useId, type InputHTMLAttributes } from "react";

export interface FieldProps extends Omit<
  InputHTMLAttributes<HTMLInputElement>,
  "id"
> {
  label: string;
  id?: string;
}

const INPUT_CLASSES =
  "rounded-md border border-slate-300 px-3 py-2 text-sm " +
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 " +
  "disabled:opacity-50 disabled:cursor-not-allowed";

/** Shared labeled single-line text input (specs/ui-consistency/plan.md step 2). */
export function Field({ label, id, className, ...rest }: FieldProps) {
  const generatedId = useId();
  const inputId = id ?? generatedId;
  const classes = className ? `${INPUT_CLASSES} ${className}` : INPUT_CLASSES;

  return (
    <label htmlFor={inputId} className="flex flex-col gap-1">
      <span>{label}</span>
      <input id={inputId} className={classes} {...rest} />
    </label>
  );
}
