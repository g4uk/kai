import { describe, it, expect, beforeEach, afterEach } from "vitest";
import {
  ApiError,
  setOnUnauthorized,
  requestOtp,
  verifyOtp,
  logout,
  listJobs,
  createJob,
  getJob,
  getAuthMe,
} from "./client";
import type { Job, JobDetail } from "./types";

// Step 2 (plan.md): API client — typed fetch wrapper + domain types.
// Fetch is stubbed by hand per call (frontend skill rule 3: "External APIs:
// interface + fake inside the package"), no mocking library.

const originalFetch = globalThis.fetch;

interface FetchCall {
  url: string;
  init?: RequestInit;
}

interface StubbedResponse {
  status: number;
  jsonBody?: unknown;
  textBody?: string;
}

function stubFetch(response: StubbedResponse): { calls: FetchCall[] } {
  const calls: FetchCall[] = [];
  globalThis.fetch = (async (
    input: RequestInfo | URL,
    init?: RequestInit,
  ): Promise<Response> => {
    calls.push({ url: String(input), init });
    const ok = response.status >= 200 && response.status < 300;
    return {
      ok,
      status: response.status,
      json: async () => response.jsonBody,
      text: async () => response.textBody ?? "",
    } as Response;
  }) as typeof fetch;
  return { calls };
}

const sampleJob: Job = {
  id: 1,
  youtube_url: "https://youtu.be/abc123",
  status: "pending",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

const sampleJobDetail: JobDetail = {
  ...sampleJob,
  id: 3,
  status: "done",
  participants: [],
  summary: "a summary",
};

describe("api client", () => {
  beforeEach(() => {
    setOnUnauthorized(() => {});
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  describe("success paths resolve with parsed data", () => {
    it("requestOtp resolves on a 202 with no body", async () => {
      const { calls } = stubFetch({ status: 202 });

      await expect(requestOtp("+15551234567")).resolves.toBeUndefined();
      expect(calls).toHaveLength(1);
      expect(calls[0].url).toContain("/api/auth/otp/request");
    });

    it("verifyOtp resolves on a 200", async () => {
      const { calls } = stubFetch({ status: 200 });

      await expect(
        verifyOtp("+15551234567", "123456"),
      ).resolves.toBeUndefined();
      expect(calls[0].url).toContain("/api/auth/otp/verify");
    });

    it("logout resolves on a 200", async () => {
      const { calls } = stubFetch({ status: 200 });

      await expect(logout()).resolves.toBeUndefined();
      expect(calls[0].url).toContain("/api/auth/logout");
    });

    it("listJobs resolves with the parsed jobs array", async () => {
      const { calls } = stubFetch({
        status: 200,
        jsonBody: { jobs: [sampleJob] },
      });

      await expect(listJobs()).resolves.toEqual([sampleJob]);
      expect(calls[0].url).toContain("/api/jobs");
    });

    it("listJobs resolves with an empty array when the API returns zero jobs", async () => {
      stubFetch({ status: 200, jsonBody: { jobs: [] } });

      await expect(listJobs()).resolves.toEqual([]);
    });

    it("createJob resolves with the created Job on a 201", async () => {
      const { calls } = stubFetch({ status: 201, jsonBody: sampleJob });

      await expect(createJob(sampleJob.youtube_url)).resolves.toEqual(
        sampleJob,
      );
      expect(calls[0].url).toContain("/api/jobs");
      expect(calls[0].init?.method).toBe("POST");
    });

    it("getJob resolves with the parsed JobDetail on a 200", async () => {
      const { calls } = stubFetch({ status: 200, jsonBody: sampleJobDetail });

      await expect(getJob(sampleJobDetail.id)).resolves.toEqual(
        sampleJobDetail,
      );
      expect(calls[0].url).toContain(`/api/jobs/${sampleJobDetail.id}`);
    });

    it("getAuthMe resolves on a 204 with no body", async () => {
      const { calls } = stubFetch({ status: 204 });

      await expect(getAuthMe()).resolves.toBeUndefined();
      expect(calls[0].url).toContain("/api/auth/me");
    });
  });

  describe("plain-text error bodies (not JSON) reject with an ApiError", () => {
    const cases: Array<{ name: string; status: number; text: string }> = [
      {
        name: "400 invalid youtube_url",
        status: 400,
        text: "invalid youtube_url",
      },
      { name: "409 duplicate job", status: 409, text: "duplicate job" },
      { name: "429 too many requests", status: 429, text: "too many requests" },
    ];

    for (const tc of cases) {
      it(`a ${tc.name} response rejects with an ApiError whose message is the exact plain-text body`, async () => {
        stubFetch({ status: tc.status, textBody: tc.text });

        const rejection = createJob("https://youtu.be/xyz789");
        await expect(rejection).rejects.toBeInstanceOf(ApiError);
        await expect(rejection).rejects.toMatchObject({
          status: tc.status,
          message: tc.text,
        });
      });
    }

    it("does not crash trying to JSON-parse a plain-text body", async () => {
      stubFetch({ status: 400, textBody: "invalid youtube_url" });

      const err = await createJob("bad").catch((e: unknown) => e);
      expect(err).toBeInstanceOf(ApiError);
      expect((err as ApiError).message).not.toContain("[object Object]");
      expect((err as ApiError).message).not.toMatch(/^\s*[{[]/);
    });
  });

  describe("getAuthMe rejects with an ApiError on a 401", () => {
    it("getAuthMe rejects with an ApiError{status:401} when no session cookie resolves", async () => {
      stubFetch({ status: 401, textBody: "unauthorized" });

      const rejection = getAuthMe();
      await expect(rejection).rejects.toBeInstanceOf(ApiError);
      await expect(rejection).rejects.toMatchObject({
        status: 401,
        message: "unauthorized",
      });
    });
  });

  describe("global 401 handling", () => {
    it("rejects with an ApiError and invokes the registered onUnauthorized callback exactly once", async () => {
      let calls = 0;
      setOnUnauthorized(() => {
        calls += 1;
      });
      stubFetch({ status: 401, textBody: "unauthorized" });

      await expect(listJobs()).rejects.toBeInstanceOf(ApiError);
      expect(calls).toBe(1);
    });

    it("does not invoke onUnauthorized for a non-401 error", async () => {
      let calls = 0;
      setOnUnauthorized(() => {
        calls += 1;
      });
      stubFetch({ status: 400, textBody: "invalid youtube_url" });

      await expect(createJob("bad")).rejects.toBeInstanceOf(ApiError);
      expect(calls).toBe(0);
    });
  });

  describe("request shape", () => {
    it("sends credentials: same-origin so the session cookie rides along", async () => {
      const { calls } = stubFetch({ status: 200, jsonBody: { jobs: [] } });

      await listJobs();

      expect(calls[0].init?.credentials).toBe("same-origin");
    });
  });
});
