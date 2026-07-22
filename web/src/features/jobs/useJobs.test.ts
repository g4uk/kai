import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { useJobs } from "./useJobs";
import type { Job } from "../../api/types";

// Step 5 (plan.md): useJobs wraps listJobs() with loading/error/data state.

const { listJobs } = vi.hoisted(() => ({ listJobs: vi.fn() }));
vi.mock("../../api/client", () => ({ listJobs }));

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
];

describe("useJobs", () => {
  beforeEach(() => {
    listJobs.mockReset();
  });

  it("starts in a loading state", () => {
    listJobs.mockReturnValue(new Promise(() => {}));

    const { result } = renderHook(() => useJobs());

    expect(result.current.loading).toBe(true);
    expect(result.current.jobs).toEqual([]);
    expect(result.current.error).toBeNull();
  });

  it("exposes the resolved jobs and clears loading", async () => {
    listJobs.mockResolvedValue(sampleJobs);

    const { result } = renderHook(() => useJobs());

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.jobs).toEqual(sampleJobs);
    expect(result.current.error).toBeNull();
  });

  it("exposes an error message and clears loading when listJobs rejects", async () => {
    listJobs.mockRejectedValue(new Error("list failed"));

    const { result } = renderHook(() => useJobs());

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.error).toBe("list failed");
    expect(result.current.jobs).toEqual([]);
  });
});
