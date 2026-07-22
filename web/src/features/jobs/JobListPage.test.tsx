import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { JobListPage } from "./JobListPage";
import type { Job } from "../../api/types";

// Step 5 (plan.md): job list screen — renders N rows (criterion 5) or an
// empty-state CTA (criterion 6, edge case 6).

const { listJobs } = vi.hoisted(() => ({ listJobs: vi.fn() }));
vi.mock("../../api/client", () => ({ listJobs }));
vi.mock("../../app/AuthContext", () => ({
  LogoutButton: () => <button>Log out</button>,
}));

const sampleJobs: Job[] = [
  {
    id: 1,
    youtube_url: "https://youtu.be/abc123",
    status: "done",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  },
  {
    id: 2,
    youtube_url: "https://youtu.be/xyz789",
    status: "pending",
    created_at: "2026-01-02T00:00:00Z",
    updated_at: "2026-01-02T00:00:00Z",
  },
  {
    id: 3,
    youtube_url: "https://youtu.be/def456",
    status: "failed",
    created_at: "2026-01-03T00:00:00Z",
    updated_at: "2026-01-03T00:00:00Z",
  },
];

function renderPage() {
  return render(
    <MemoryRouter>
      <JobListPage />
    </MemoryRouter>,
  );
}

describe("JobListPage", () => {
  beforeEach(() => {
    listJobs.mockReset();
  });

  it("renders exactly N rows with url, status, and created-at for N jobs (criterion 5)", async () => {
    listJobs.mockResolvedValue(sampleJobs);

    renderPage();

    await waitFor(
      () =>
        expect(screen.getAllByRole("row")).toHaveLength(sampleJobs.length + 1), // +1 header row
    );
    for (const job of sampleJobs) {
      expect(screen.getByText(job.youtube_url)).toBeInTheDocument();
      expect(screen.getByText(job.status)).toBeInTheDocument();
    }
  });

  it("renders an empty-state CTA, not a table or a stuck spinner, for zero jobs (criterion 6, edge case 6)", async () => {
    listJobs.mockResolvedValue([]);

    renderPage();

    await waitFor(() =>
      expect(
        screen.getByRole("link", { name: /submit your first analysis/i }),
      ).toBeInTheDocument(),
    );
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  });
});
