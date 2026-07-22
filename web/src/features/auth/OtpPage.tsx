import { useState, type FormEvent } from "react";

export interface OtpPageProps {
  phone: string;
  error: string | null;
  isSubmitting: boolean;
  onSubmit: (code: string) => void;
}

/** Presentational OTP-code entry screen (specs/ui/plan.md step 3). */
export function OtpPage({
  phone,
  error,
  isSubmitting,
  onSubmit,
}: OtpPageProps) {
  const [code, setCode] = useState("");

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isSubmitting) {
      return;
    }
    onSubmit(code);
  }

  return (
    <div className="flex flex-col gap-4">
      <p>A code was sent to {phone}.</p>
      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        <label className="flex flex-col gap-1">
          <span>Code</span>
          <input
            type="text"
            value={code}
            onChange={(event) => setCode(event.target.value)}
            disabled={isSubmitting}
          />
        </label>
        {error !== null && <p role="alert">{error}</p>}
        <button type="submit" disabled={isSubmitting}>
          Verify
        </button>
      </form>
    </div>
  );
}
