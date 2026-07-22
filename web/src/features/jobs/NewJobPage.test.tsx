import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { NewJobPage } from "./NewJobPage";
import type { Job } from "../../api/types";

// Step 6 (plan.md): new-analysis submission form — client-side validation
// gate (criterion 8), successful submission navigates to the results route
// (criterion 7), server-side error surfaces inline without navigating
// (criterion 9).

const { createJob } = vi.hoisted(() => ({ createJob: vi.fn() }));
vi.mock("../../api/client", () => ({ createJob }));

const createdJob: Job = {
  id: 42,
  youtube_url: "https://youtu.be/dQw4w9WgXcQ",
  status: "pending",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

function renderPage() {
  return render(
    <MemoryRouter initialEntries={["/jobs/new"]}>
      <Routes>
        <Route path="/jobs/new" element={<NewJobPage />} />
        <Route path="/jobs/:id" element={<div>Job Results Screen</div>} />
      </Routes>
    </MemoryRouter>,
  );
}

async function submitUrl(
  user: ReturnType<typeof userEvent.setup>,
  url: string,
) {
  await user.type(screen.getByLabelText(/youtube url/i), url);
  await user.click(screen.getByRole("button", { name: /submit/i }));
}

describe("NewJobPage", () => {
  beforeEach(() => {
    createJob.mockReset();
  });

  it("blocks submission of a malformed URL, shows an inline error, and never calls createJob (criterion 8)", async () => {
    const user = userEvent.setup();
    renderPage();

    await submitUrl(user, "not-a-youtube-url");

    expect(screen.getByText(/enter a valid youtube url/i)).toBeInTheDocument();
    expect(createJob).not.toHaveBeenCalled();
  });

  it("navigates to the new job's results screen on a 2xx response (criterion 7)", async () => {
    createJob.mockResolvedValue(createdJob);
    const user = userEvent.setup();
    renderPage();

    await submitUrl(user, createdJob.youtube_url);

    expect(createJob).toHaveBeenCalledExactlyOnceWith(createdJob.youtube_url);
    await waitFor(() =>
      expect(screen.getByText("Job Results Screen")).toBeInTheDocument(),
    );
  });

  it("shows the server's plain-text error inline and stays on the form on a non-2xx response (criterion 9)", async () => {
    createJob.mockRejectedValue(new Error("duplicate job"));
    const user = userEvent.setup();
    renderPage();

    await submitUrl(user, "https://youtu.be/dQw4w9WgXcQ");

    expect(await screen.findByText("duplicate job")).toBeInTheDocument();
    expect(screen.getByLabelText(/youtube url/i)).toBeInTheDocument();
    expect(screen.queryByText("Job Results Screen")).not.toBeInTheDocument();
  });
});
