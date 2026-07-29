import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { AuthProvider, LogoutButton, useAuth } from "../../app/AuthContext";
import { useJobStatusEvents } from "./useJobStatusEvents";

// Step 7 (plan.md): useJobStatusEvents — opens a credentialed EventSource at
// /api/jobs/stream while useAuth().status === "authenticated", forwards
// job_status messages to useToast().addToast(...), and closes the
// connection whenever status moves away from "authenticated" (criterion 6).
//
// jsdom has no native EventSource (plan.md's top-of-file risk note) — every
// test here stubs a small test-local MockEventSource via vi.stubGlobal.

const { getAuthMe, logout, setOnUnauthorized, ApiErrorMock } = vi.hoisted(
  () => {
    class ApiErrorMock extends Error {
      status: number;
      constructor(status: number, message: string) {
        super(message);
        this.status = status;
        this.name = "ApiError";
      }
    }
    return {
      getAuthMe: vi.fn(),
      logout: vi.fn(),
      setOnUnauthorized: vi.fn(),
      ApiErrorMock,
    };
  },
);

vi.mock("../../api/client", () => ({
  getAuthMe,
  logout,
  setOnUnauthorized,
  ApiError: ApiErrorMock,
}));

// Test-local spy standing in for the real web/src/ui/Toast.tsx (step 6),
// per the plan's "test-local ToastProvider/spy" option — decouples this
// hook's tests from Toast's own rendering/timing behavior (covered by
// Toast.test.tsx). App.test.tsx separately proves the real ToastProvider is
// wired end-to-end.
const { addToast } = vi.hoisted(() => ({ addToast: vi.fn() }));
vi.mock("../../ui/Toast", () => ({
  useToast: () => ({ addToast }),
}));

class MockEventSource {
  static instances: MockEventSource[] = [];
  url: string;
  closed = false;
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
    this.closed = true;
  }

  /** Test helper: fires `type` with a MessageEvent-shaped payload. */
  dispatch(type: string, data: unknown): void {
    const event = { data: JSON.stringify(data) };
    for (const handler of this.listeners[type] ?? []) {
      handler(event);
    }
  }
}

function Notifier() {
  useJobStatusEvents();
  return null;
}

function StatusProbe() {
  const { status } = useAuth();
  return <div data-testid="status">{status}</div>;
}

function renderNotifier(initialEntries: string[] = ["/"]) {
  return render(
    <MemoryRouter initialEntries={initialEntries}>
      <AuthProvider>
        <Notifier />
        <StatusProbe />
        <LogoutButton />
      </AuthProvider>
    </MemoryRouter>,
  );
}

describe("useJobStatusEvents", () => {
  beforeEach(() => {
    getAuthMe.mockReset();
    logout.mockReset();
    setOnUnauthorized.mockReset();
    addToast.mockReset();
    MockEventSource.instances = [];
    vi.stubGlobal("EventSource", MockEventSource);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("does not construct an EventSource while unauthenticated", async () => {
    getAuthMe.mockRejectedValue(new ApiErrorMock(401, "unauthorized"));

    renderNotifier();

    await waitFor(() =>
      expect(screen.getByTestId("status")).toHaveTextContent("anonymous"),
    );
    expect(MockEventSource.instances).toHaveLength(0);
  });

  it("constructs exactly one EventSource at /api/jobs/stream once authenticated", async () => {
    getAuthMe.mockResolvedValue(undefined);

    renderNotifier();

    await waitFor(() =>
      expect(screen.getByTestId("status")).toHaveTextContent("authenticated"),
    );
    await waitFor(() => expect(MockEventSource.instances).toHaveLength(1));
    expect(MockEventSource.instances[0].url).toBe("/api/jobs/stream");
  });

  it("distinguishes terminal (done/failed) status text from in-progress (processing) text via addToast (criterion 4)", async () => {
    getAuthMe.mockResolvedValue(undefined);

    renderNotifier();

    await waitFor(() => expect(MockEventSource.instances).toHaveLength(1));
    const source = MockEventSource.instances[0];

    source.dispatch("job_status", { job_id: 1, status: "processing" });
    source.dispatch("job_status", { job_id: 2, status: "done" });
    source.dispatch("job_status", { job_id: 3, status: "failed" });

    await waitFor(() => expect(addToast).toHaveBeenCalledTimes(3));

    const [processingText] = addToast.mock.calls[0];
    const [doneText] = addToast.mock.calls[1];
    const [failedText] = addToast.mock.calls[2];

    expect(typeof processingText).toBe("string");
    expect(doneText).not.toBe(processingText);
    expect(failedText).not.toBe(processingText);
  });

  it("closes the EventSource when auth status transitions from authenticated to anonymous (logout, criterion 6)", async () => {
    getAuthMe.mockResolvedValue(undefined);
    logout.mockResolvedValue(undefined);
    const user = userEvent.setup();

    renderNotifier();

    await waitFor(() => expect(MockEventSource.instances).toHaveLength(1));
    const source = MockEventSource.instances[0];
    expect(source.closed).toBe(false);

    await user.click(screen.getByRole("button", { name: /log out/i }));

    await waitFor(() => expect(source.closed).toBe(true));
  });
});
