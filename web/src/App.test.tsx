import { describe, it, expect, vi, beforeEach } from "vitest";
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

const { getAuthMe, ApiErrorMock } = vi.hoisted(() => {
  class ApiErrorMock extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.status = status;
      this.name = "ApiError";
    }
  }
  return { getAuthMe: vi.fn(), ApiErrorMock };
});

vi.mock("./api/client", () => ({
  listJobs: vi.fn(),
  getJob: vi.fn(),
  getAuthMe,
  requestOtp: vi.fn(),
  verifyOtp: vi.fn(),
  logout: vi.fn(),
  createJob: vi.fn(),
  setOnUnauthorized: vi.fn(),
  ApiError: ApiErrorMock,
}));

describe("App", () => {
  beforeEach(() => {
    getAuthMe.mockReset();
    window.history.pushState({}, "", "/");
  });

  it("renders the login (phone-number) screen at the root route when unauthenticated", async () => {
    getAuthMe.mockRejectedValue(new ApiErrorMock(401, "unauthorized"));

    render(<App />);

    await waitFor(() =>
      expect(screen.getByLabelText(/phone number/i)).toBeInTheDocument(),
    );
  });
});
