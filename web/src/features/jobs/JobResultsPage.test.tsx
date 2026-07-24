import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
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
vi.mock("../../app/AuthContext", () => ({
  LogoutButton: () => <button>Log out</button>,
}));

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

    // Both participants share the "strikes_landed" key (on purpose, to
    // exercise generic rendering per participant) — scope queries to each
    // participant's own section rather than querying document-wide, since
    // that key/value legitimately appears twice in the DOM.
    const sectionA = within(
      screen.getByRole("region", { name: "Participant A" }),
    );
    expect(sectionA.getByText("strikes_landed")).toBeInTheDocument();
    expect(sectionA.getByText("12")).toBeInTheDocument();
    expect(sectionA.getByText("advantage_points")).toBeInTheDocument();
    expect(sectionA.getByText("3")).toBeInTheDocument();

    const sectionB = within(
      screen.getByRole("region", { name: "Participant B" }),
    );
    expect(sectionB.getByText("strikes_landed")).toBeInTheDocument();
    expect(sectionB.getByText("5")).toBeInTheDocument();

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

  it("renders a Log out action via the shared PageHeader for a done job (criterion 7, plan step 8)", async () => {
    getJob.mockResolvedValue(baseJob({ status: "done" }));

    renderPage();

    expect(
      await screen.findByRole("button", { name: /log out/i }),
    ).toBeInTheDocument();
  });
});
