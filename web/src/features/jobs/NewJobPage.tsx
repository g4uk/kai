import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { createJob } from "../../api/client";
import { isValidYoutubeUrl } from "./youtubeUrl";

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
      <h1 className="text-xl font-semibold">Submit a new analysis</h1>
      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        <label className="flex flex-col gap-1">
          <span>YouTube URL</span>
          <input
            type="text"
            value={url}
            onChange={(event) => setUrl(event.target.value)}
          />
        </label>
        {validationError !== null && <p role="alert">{validationError}</p>}
        {serverError !== null && <p role="alert">{serverError}</p>}
        <button type="submit" disabled={submitting}>
          Submit
        </button>
      </form>
    </div>
  );
}
