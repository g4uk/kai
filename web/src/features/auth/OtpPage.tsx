import { useState, type FormEvent } from "react";
import { Field } from "../../ui/Field";
import { Button } from "../../ui/Button";
import { Alert } from "../../ui/Alert";

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
    <div className="max-w-sm mx-auto flex flex-col gap-4 p-4">
      <p>A code was sent to {phone}.</p>
      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        <Field
          label="Code"
          type="text"
          value={code}
          onChange={(event) => setCode(event.target.value)}
          disabled={isSubmitting}
        />
        <Alert>{error}</Alert>
        <Button type="submit" disabled={isSubmitting}>
          Verify
        </Button>
      </form>
    </div>
  );
}
