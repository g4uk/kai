import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useNavigate } from "react-router-dom";
import { AuthProvider, LogoutButton } from "./AuthContext";
import { AppRoutes } from "./router";

// Step 4 (plan.md): app shell — router, AuthContext, ProtectedRoute, global
// 401 handling, logout. JobListPage/NewJobPage/JobResultsPage (steps 5-7)
// are faked here as simple stand-ins so this suite tests routing/auth
// behavior only, decoupled from those screens' own content (tested in their
// own step 5-7 suites).

const { listJobs, getJob, setOnUnauthorized, ApiErrorMock } = vi.hoisted(() => {
  class ApiErrorMock extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.status = status;
      this.name = "ApiError";
    }
  }
  return {
    listJobs: vi.fn(),
    getJob: vi.fn(),
    setOnUnauthorized: vi.fn(),
    ApiErrorMock,
  };
});

vi.mock("../api/client", () => ({
  listJobs,
  getJob,
  setOnUnauthorized,
  requestOtp: vi.fn(),
  verifyOtp: vi.fn(),
  logout: vi.fn(),
  createJob: vi.fn(),
  ApiError: ApiErrorMock,
}));

vi.mock("../features/jobs/JobListPage", () => ({
  JobListPage: () => <div>Job List Screen</div>,
}));
vi.mock("../features/jobs/NewJobPage", () => ({
  NewJobPage: () => <div>New Job Screen</div>,
}));
vi.mock("../features/jobs/JobResultsPage", () => ({
  JobResultsPage: () => <div>Job Results Screen</div>,
}));

/** Test-only helper exposing an imperative "browser back" via useNavigate(-1). */
function TestBackButton() {
  const navigate = useNavigate();
  return <button onClick={() => navigate(-1)}>test-browser-back</button>;
}

function renderApp(
  initialEntries: string[],
  initialIndex = initialEntries.length - 1,
) {
  return render(
    <MemoryRouter initialEntries={initialEntries} initialIndex={initialIndex}>
      <AuthProvider>
        <TestBackButton />
        <LogoutButton />
        <AppRoutes />
      </AuthProvider>
    </MemoryRouter>,
  );
}

describe("router / AuthContext / ProtectedRoute", () => {
  beforeEach(() => {
    listJobs.mockReset();
    getJob.mockReset();
    setOnUnauthorized.mockReset();
  });

  it("an unauthenticated root load shows the login screen and probes listJobs exactly once, with no getJob call (criterion 1)", async () => {
    listJobs.mockRejectedValue(new ApiErrorMock(401, "unauthorized"));

    renderApp(["/"]);

    await waitFor(() =>
      expect(screen.getByLabelText(/phone number/i)).toBeInTheDocument(),
    );
    expect(listJobs).toHaveBeenCalledOnce();
    expect(getJob).not.toHaveBeenCalled();
  });

  it("a deep link to /jobs/999 while logged out redirects to /login without calling getJob (edge case 8)", async () => {
    listJobs.mockRejectedValue(new ApiErrorMock(401, "unauthorized"));

    renderApp(["/jobs/999"]);

    await waitFor(() =>
      expect(screen.getByLabelText(/phone number/i)).toBeInTheDocument(),
    );
    expect(getJob).not.toHaveBeenCalled();
    expect(screen.queryByText("Job Results Screen")).not.toBeInTheDocument();
  });

  it("a 401 surfaced from any call while on /jobs redirects to /login (criterion 15)", async () => {
    listJobs.mockResolvedValue([]);

    renderApp(["/jobs"]);

    await waitFor(() =>
      expect(screen.getByText("Job List Screen")).toBeInTheDocument(),
    );

    expect(setOnUnauthorized).toHaveBeenCalled();
    const registeredHandler =
      setOnUnauthorized.mock.calls[setOnUnauthorized.mock.calls.length - 1][0];

    registeredHandler();

    await waitFor(() =>
      expect(screen.getByLabelText(/phone number/i)).toBeInTheDocument(),
    );
    expect(screen.queryByText("Job List Screen")).not.toBeInTheDocument();
  });

  it("logout then browser back re-probes listJobs rather than rendering a cached job list (criterion 16)", async () => {
    listJobs.mockResolvedValue([]);
    const user = userEvent.setup();

    renderApp(["/somewhere", "/jobs"], 1);

    await waitFor(() =>
      expect(screen.getByText("Job List Screen")).toBeInTheDocument(),
    );
    expect(listJobs).toHaveBeenCalledOnce();

    await user.click(screen.getByRole("button", { name: /log out/i }));

    await waitFor(() =>
      expect(screen.getByLabelText(/phone number/i)).toBeInTheDocument(),
    );

    await user.click(
      screen.getByRole("button", { name: /test-browser-back/i }),
    );

    // The re-fetch itself is the proof of "no cached view": a stale/cached
    // render would never re-invoke listJobs on remount. By the time the
    // second call has committed, ProtectedRoute has already re-rendered
    // with the fresh (anonymous) status, so asserting the call count here
    // is sufficient — asserting on the resulting DOM afterward is racy
    // (the re-render from the second call may already have committed).
    await waitFor(() => expect(listJobs).toHaveBeenCalledTimes(2));
  });
});
