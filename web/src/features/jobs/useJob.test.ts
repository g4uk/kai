import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { useJob } from "./useJob";
import type { JobDetail } from "../../api/types";

// Step 7 (plan.md): useJob(id) wraps getJob(id) with loading/error/data
// state, exposing a 404 ApiError distinctly enough for the page to branch
// on (criterion 14).

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

const sampleDetail: JobDetail = {
  id: 3,
  youtube_url: "https://youtu.be/abc123",
  status: "done",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
  participants: [],
  summary: "a summary",
};

describe("useJob", () => {
  beforeEach(() => {
    getJob.mockReset();
  });

  it("starts in a loading state", () => {
    getJob.mockReturnValue(new Promise(() => {}));

    const { result } = renderHook(() => useJob(3));

    expect(result.current.loading).toBe(true);
    expect(result.current.job).toBeNull();
    expect(result.current.error).toBeNull();
  });

  it("exposes the resolved job detail and clears loading", async () => {
    getJob.mockResolvedValue(sampleDetail);

    const { result } = renderHook(() => useJob(3));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.job).toEqual(sampleDetail);
    expect(result.current.error).toBeNull();
  });

  it("exposes a distinct 404 ApiError so the page can render a not-found state (criterion 14)", async () => {
    getJob.mockRejectedValue(new ApiErrorMock(404, "not found"));

    const { result } = renderHook(() => useJob(999));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.error).toBeInstanceOf(ApiErrorMock);
    expect(
      (result.current.error as InstanceType<typeof ApiErrorMock>).status,
    ).toBe(404);
    expect(result.current.job).toBeNull();
  });
});
