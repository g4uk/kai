import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import App from "./App";

// Reviewer-found gap fix: App.tsx was still the step-1 scaffold placeholder
// and main.tsx never mounted the router/AuthProvider/route tree built in
// step 4, so a real page load showed only a static heading with no login
// form and no routes. Every other suite (router.test.tsx, LoginPage.test.tsx,
// etc.) renders pieces in isolation wrapped in its own MemoryRouter/
// AuthProvider, so none of them exercise the real, unmodified `App` export
// the way `main.tsx` actually does. This test renders `<App />` itself (no
// MemoryRouter, no manual AuthProvider wiring) to prove App.tsx really
// composes BrowserRouter + AuthProvider + AppRoutes end-to-end.

const { getAuthMe, listJobs, ApiErrorMock } = vi.hoisted(() => {
  class ApiErrorMock extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.status = status;
      this.name = "ApiError";
    }
  }
  return { getAuthMe: vi.fn(), listJobs: vi.fn(), ApiErrorMock };
});

vi.mock("./api/client", () => ({
  listJobs,
  getJob: vi.fn(),
  getAuthMe,
  requestOtp: vi.fn(),
  verifyOtp: vi.fn(),
  logout: vi.fn(),
  createJob: vi.fn(),
  setOnUnauthorized: vi.fn(),
  ApiError: ApiErrorMock,
}));

// specs/popup-notifications+sse/spec.md: jsdom has no native EventSource
// (plan.md's top-of-file risk note) — stub a minimal test-local mock class
// that records constructor calls and lets the test manually fire a
// job_status message.
class MockEventSource {
  static instances: MockEventSource[] = [];
  url: string;
  private listeners: Record<string, Array<(event: { data: string }) => void>> =
    {};

  constructor(url: string) {
    this.url = url;
    MockEventSource.instances.push(this);
  }

  addEventListener(
    type: string,
    handler: (event: { data: string }) => void,
  ): void {
    (this.listeners[type] ??= []).push(handler);
  }

  removeEventListener(): void {
    // not exercised by these tests
  }

  close(): void {
    // not exercised by these tests
  }

  dispatch(type: string, data: unknown): void {
    const event = { data: JSON.stringify(data) };
    for (const handler of this.listeners[type] ?? []) {
      handler(event);
    }
  }
}

describe("App", () => {
  beforeEach(() => {
    getAuthMe.mockReset();
    listJobs.mockReset();
    window.history.pushState({}, "", "/");
  });

  it("renders the login (phone-number) screen at the root route when unauthenticated", async () => {
    getAuthMe.mockRejectedValue(new ApiErrorMock(401, "unauthorized"));

    render(<App />);

    await waitFor(() =>
      expect(screen.getByLabelText(/phone number/i)).toBeInTheDocument(),
    );
  });

  // specs/popup-notifications+sse/spec.md, plan.md step 7: proves App.tsx
  // actually wires ToastProvider + the SSE hook (useJobStatusEvents) +
  // AppRoutes together — a job_status event fired on the app-constructed
  // EventSource must produce a visible toast somewhere in the rendered app
  // shell, not just in the hook's own isolated test.
  describe("popup notifications wiring", () => {
    beforeEach(() => {
      MockEventSource.instances = [];
      vi.stubGlobal("EventSource", MockEventSource);
    });

    afterEach(() => {
      vi.unstubAllGlobals();
    });

    it("shows a toast when a job_status SSE event arrives, proving ToastProvider + useJobStatusEvents are wired into the real app shell", async () => {
      getAuthMe.mockResolvedValue(undefined);
      listJobs.mockResolvedValue([]);

      render(<App />);

      await waitFor(() =>
        expect(MockEventSource.instances.length).toBeGreaterThan(0),
      );
      const source = MockEventSource.instances[0];
      expect(source.url).toBe("/api/jobs/stream");

      source.dispatch("job_status", { job_id: 1, status: "done" });

      await waitFor(() =>
        expect(screen.getByText(/done/i)).toBeInTheDocument(),
      );
    });

    // Review fix (minor #6): both ProtectedRoute (router.tsx) and
    // JobStatusNotifier (App.tsx, via useJobStatusEvents) call
    // ensureChecked() once a protected route is mounted alongside the app
    // root — proves AuthContext.tsx's `checking.current` in-flight guard
    // still dedupes them into a single GET /auth/me call, matching the
    // pattern of router.test.tsx's criterion-1 "exactly once" assertion.
    it("calls getAuthMe exactly once on initial load of a protected route, even though both ProtectedRoute and JobStatusNotifier call ensureChecked", async () => {
      getAuthMe.mockResolvedValue(undefined);
      listJobs.mockResolvedValue([]);
      window.history.pushState({}, "", "/jobs");

      render(<App />);

      await waitFor(() => expect(screen.getByText("Jobs")).toBeInTheDocument());
      await waitFor(() =>
        expect(MockEventSource.instances.length).toBeGreaterThan(0),
      );

      expect(getAuthMe).toHaveBeenCalledOnce();
    });
  });
});
