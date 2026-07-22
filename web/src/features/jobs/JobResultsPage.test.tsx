import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { JobResultsPage } from "./JobResultsPage";
import type { JobDetail } from "../../api/types";

// Step 7 (plan.md): results screen — branches purely on job status.
// pending/processing -> in-progress, no metrics section (criterion 12).
// done -> one section per participant, generic key/value rendering, "no
// metrics recorded" note for empty participants (criterion 11).
// failed -> failure state with summary or a generic fallback (edge case 4).
// 404 -> generic not-found state (criterion 14).

const { getJob, ApiErrorMock } = vi.hoisted(() => {
  class ApiErrorMock extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.status = status;
      this.name = "ApiError";
    }
  }
  return { getJob: vi.fn(), ApiErrorMock };
});
vi.mock("../../api/client", () => ({ getJob, ApiError: ApiErrorMock }));

function baseJob(overrides: Partial<JobDetail>): JobDetail {
  return {
    id: 3,
    youtube_url: "https://youtu.be/abc123",
    status: "done",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    participants: [],
    summary: null,
    ...overrides,
  };
}

function renderPage(id = "3") {
  return render(
    <MemoryRouter initialEntries={[`/jobs/${id}`]}>
      <Routes>
        <Route path="/jobs/:id" element={<JobResultsPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("JobResultsPage", () => {
  beforeEach(() => {
    getJob.mockReset();
  });

  it.each(["pending", "processing"])(
    "shows an in-progress state and renders no metrics section for a %s job (criterion 12, edge case 3)",
    async (status) => {
      getJob.mockResolvedValue(baseJob({ status }));

      renderPage();

      expect(await screen.findByText(/in progress/i)).toBeInTheDocument();
      expect(screen.queryByRole("region")).not.toBeInTheDocument();
      expect(
        screen.queryByText(/no metrics recorded/i),
      ).not.toBeInTheDocument();
    },
  );

  it("renders one section per participant with every metric key/value pair, plus the summary, for a done job (criterion 10)", async () => {
    getJob.mockResolvedValue(
      baseJob({
        status: "done",
        summary: "Participant A won by ippon.",
        participants: [
          {
            id: 1,
            label: "Participant A",
            metrics: [
              { key: "strikes_landed", value: 12 },
              { key: "advantage_points", value: 3 },
            ],
          },
          {
            id: 2,
            label: "Participant B",
            metrics: [{ key: "strikes_landed", value: 5 }],
          },
        ],
      }),
    );

    renderPage();

    expect(await screen.findByText("Participant A")).toBeInTheDocument();
    expect(screen.getByText("Participant B")).toBeInTheDocument();
    expect(screen.getByText("strikes_landed")).toBeInTheDocument();
    expect(screen.getByText("12")).toBeInTheDocument();
    expect(screen.getByText("advantage_points")).toBeInTheDocument();
    expect(screen.getByText("3")).toBeInTheDocument();
    expect(screen.getByText("5")).toBeInTheDocument();
    expect(screen.getByText("Participant A won by ippon.")).toBeInTheDocument();
  });

  it("renders an explicit 'no metrics recorded' note for a participant with zero metrics (criterion 11, edge case 5)", async () => {
    getJob.mockResolvedValue(
      baseJob({
        status: "done",
        participants: [{ id: 1, label: "Participant A", metrics: [] }],
      }),
    );

    renderPage();

    expect(await screen.findByText("Participant A")).toBeInTheDocument();
    expect(screen.getByText(/no metrics recorded/i)).toBeInTheDocument();
  });

  it("shows the failure state with the summary as error detail for a failed job", async () => {
    getJob.mockResolvedValue(
      baseJob({ status: "failed", summary: "video unavailable" }),
    );

    renderPage();

    expect(await screen.findByText("video unavailable")).toBeInTheDocument();
    expect(screen.queryByText("Participant A")).not.toBeInTheDocument();
  });

  it("shows a generic fallback message (not 'undefined'/'null') for a failed job with no summary (edge case 4)", async () => {
    getJob.mockResolvedValue(baseJob({ status: "failed", summary: null }));

    renderPage();

    await waitFor(() => expect(getJob).toHaveBeenCalled());
    expect(screen.queryByText("null")).not.toBeInTheDocument();
    expect(screen.queryByText("undefined")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toMatch(/\bundefined\b/);
    expect(document.body.textContent).not.toMatch(/\bnull\b/);
    expect(
      await screen.findByText(/something went wrong/i),
    ).toBeInTheDocument();
  });

  it("renders a generic not-found state for a 404, never revealing whether the job exists (criterion 14)", async () => {
    getJob.mockRejectedValue(new ApiErrorMock(404, "not found"));

    renderPage("999");

    expect(await screen.findByText(/not found/i)).toBeInTheDocument();
  });
});
