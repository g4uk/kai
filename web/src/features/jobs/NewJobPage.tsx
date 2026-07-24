import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { createJob } from "../../api/client";
import { isValidYoutubeUrl } from "./youtubeUrl";
import { LogoutButton } from "../../app/AuthContext";
import { PageHeader } from "../../ui/PageHeader";
import { Field } from "../../ui/Field";
import { Button } from "../../ui/Button";
import { Alert } from "../../ui/Alert";

function messageOf(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return String(err);
}

/** New-analysis submission form (specs/ui/plan.md step 6). */
export function NewJobPage() {
  const [url, setUrl] = useState("");
  const [validationError, setValidationError] = useState<string | null>(null);
  const [serverError, setServerError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const navigate = useNavigate();

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (submitting) {
      return;
    }
    if (!isValidYoutubeUrl(url)) {
      setValidationError("Enter a valid YouTube URL.");
      return;
    }
    setValidationError(null);
    setServerError(null);
    setSubmitting(true);
    try {
      const job = await createJob(url);
      navigate(`/jobs/${job.id}`);
    } catch (err) {
      setServerError(messageOf(err));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="flex flex-col gap-4 p-4">
      <PageHeader title="Submit a new analysis">
        <LogoutButton />
      </PageHeader>
      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        <Field
          label="YouTube URL"
          type="text"
          value={url}
          onChange={(event) => setUrl(event.target.value)}
          disabled={submitting}
        />
        <Alert>{validationError}</Alert>
        <Alert>{serverError}</Alert>
        <Button type="submit" disabled={submitting}>
          Submit
        </Button>
      </form>
    </div>
  );
}
