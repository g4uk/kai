// Typed fetch wrapper — one function per endpoint, per specs/ui/plan.md step 2.
// No retry, no caching. Error bodies from the Go API are plain text, not
// JSON (see internal/handler/auth.go / jobs.go's use of http.Error), so
// non-2xx responses are surfaced via response.text(), never JSON.parse'd.

import type { Job, JobDetail } from "./types";

/** Thrown for any non-2xx API response. `message` is the exact plain-text body. */
export class ApiError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

type UnauthorizedHandler = () => void;

let onUnauthorized: UnauthorizedHandler | null = null;

/** Registers a callback invoked whenever any apiFetch call receives a 401. */
export function setOnUnauthorized(handler: UnauthorizedHandler): void {
  onUnauthorized = handler;
}

async function apiFetch(path: string, init?: RequestInit): Promise<Response> {
  const response = await fetch(`/api${path}`, {
    ...init,
    credentials: "same-origin",
  });

  if (!response.ok) {
    const message = await response.text();
    if (response.status === 401) {
      onUnauthorized?.();
    }
    throw new ApiError(response.status, message);
  }

  return response;
}

function jsonHeaders(): HeadersInit {
  return { "Content-Type": "application/json" };
}

export async function requestOtp(phoneNumber: string): Promise<void> {
  await apiFetch("/auth/otp/request", {
    method: "POST",
    headers: jsonHeaders(),
    body: JSON.stringify({ phone_number: phoneNumber }),
  });
}

export async function verifyOtp(
  phoneNumber: string,
  code: string,
): Promise<void> {
  await apiFetch("/auth/otp/verify", {
    method: "POST",
    headers: jsonHeaders(),
    body: JSON.stringify({ phone_number: phoneNumber, code }),
  });
}

export async function logout(): Promise<void> {
  await apiFetch("/auth/logout", { method: "POST" });
}

export async function listJobs(): Promise<Job[]> {
  const response = await apiFetch("/jobs");
  const body = (await response.json()) as { jobs: Job[] };
  return body.jobs;
}

export async function createJob(youtubeUrl: string): Promise<Job> {
  const response = await apiFetch("/jobs", {
    method: "POST",
    headers: jsonHeaders(),
    body: JSON.stringify({ youtube_url: youtubeUrl }),
  });
  return (await response.json()) as Job;
}

export async function getJob(id: number): Promise<JobDetail> {
  const response = await apiFetch(`/jobs/${id}`);
  return (await response.json()) as JobDetail;
}
