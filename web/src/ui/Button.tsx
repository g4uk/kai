import type { ButtonHTMLAttributes } from "react";

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: "primary";
}

const BASE_CLASSES =
  "rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white " +
  "hover:bg-blue-700 focus-visible:outline-none focus-visible:ring-2 " +
  "focus-visible:ring-blue-500 focus-visible:ring-offset-2 " +
  "disabled:opacity-50 disabled:cursor-not-allowed";

/** Shared primary-action button (specs/ui-consistency/plan.md step 1). */
export function Button({
  variant: _variant = "primary",
  className,
  ...rest
}: ButtonProps) {
  const classes = className ? `${BASE_CLASSES} ${className}` : BASE_CLASSES;
  return <button className={classes} {...rest} />;
}
