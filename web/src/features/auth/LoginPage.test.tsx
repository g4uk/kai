import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { LoginPage } from "./LoginPage";

// Step 3 (plan.md): LoginPage — phone entry, client-side E.164 shape check,
// then requestOtp, transitioning to the OTP screen on success. requestOtp/
// verifyOtp are faked at the API-client-module boundary (frontend skill
// rule 3), not raw fetch.

const { requestOtp, verifyOtp } = vi.hoisted(() => ({
  requestOtp: vi.fn(),
  verifyOtp: vi.fn(),
}));

vi.mock("../../api/client", () => ({
  requestOtp,
  verifyOtp,
}));

async function submitPhone(
  user: ReturnType<typeof userEvent.setup>,
  phone: string,
) {
  await user.type(screen.getByLabelText(/phone number/i), phone);
  await user.click(screen.getByRole("button", { name: /send code/i }));
}

describe("LoginPage", () => {
  beforeEach(() => {
    requestOtp.mockReset();
    verifyOtp.mockReset();
  });

  it("shows the phone entry screen initially", () => {
    render(<LoginPage onLoggedIn={() => {}} />);

    expect(screen.getByLabelText(/phone number/i)).toBeInTheDocument();
  });

  it("blocks submission of a malformed phone number and never calls requestOtp", async () => {
    const user = userEvent.setup();
    render(<LoginPage onLoggedIn={() => {}} />);

    await submitPhone(user, "not-a-phone-number");

    expect(screen.getByText(/enter a valid phone number/i)).toBeInTheDocument();
    expect(requestOtp).not.toHaveBeenCalled();
  });

  it("shows the OTP screen after a valid phone number is submitted (criterion 2)", async () => {
    requestOtp.mockResolvedValue(undefined);
    const user = userEvent.setup();
    render(<LoginPage onLoggedIn={() => {}} />);

    await submitPhone(user, "+15551234567");

    expect(requestOtp).toHaveBeenCalledExactlyOnceWith("+15551234567");
    expect(screen.getByLabelText(/code/i)).toBeInTheDocument();
  });

  it("shows the API's inline error and keeps the OTP form visible on an incorrect code (criterion 4)", async () => {
    requestOtp.mockResolvedValue(undefined);
    verifyOtp.mockRejectedValue(new Error("invalid or expired code"));
    const user = userEvent.setup();
    render(<LoginPage onLoggedIn={() => {}} />);

    await submitPhone(user, "+15551234567");
    await user.type(screen.getByLabelText(/code/i), "000000");
    await user.click(screen.getByRole("button", { name: /verify/i }));

    expect(screen.getByText("invalid or expired code")).toBeInTheDocument();
    expect(screen.getByLabelText(/code/i)).toBeInTheDocument();
  });

  it("navigates away via onLoggedIn on a correct code (criterion 3)", async () => {
    requestOtp.mockResolvedValue(undefined);
    verifyOtp.mockResolvedValue(undefined);
    const onLoggedIn = vi.fn();
    const user = userEvent.setup();
    render(<LoginPage onLoggedIn={onLoggedIn} />);

    await submitPhone(user, "+15551234567");
    await user.type(screen.getByLabelText(/code/i), "123456");
    await user.click(screen.getByRole("button", { name: /verify/i }));

    expect(onLoggedIn).toHaveBeenCalledOnce();
  });

  it("a rapid double-click on the phone submit button only fires one requestOtp call", async () => {
    let resolveRequest!: () => void;
    requestOtp.mockReturnValue(
      new Promise<void>((resolve) => {
        resolveRequest = resolve;
      }),
    );
    const user = userEvent.setup();
    render(<LoginPage onLoggedIn={() => {}} />);

    await user.type(screen.getByLabelText(/phone number/i), "+15551234567");
    const sendCodeButton = screen.getByRole("button", { name: /send code/i });
    await user.dblClick(sendCodeButton);

    expect(requestOtp).toHaveBeenCalledOnce();
    resolveRequest();
  });

  it("a rapid double-click on the OTP submit button only fires one verifyOtp call (edge case 7)", async () => {
    requestOtp.mockResolvedValue(undefined);
    let resolveVerify!: () => void;
    verifyOtp.mockReturnValue(
      new Promise<void>((resolve) => {
        resolveVerify = resolve;
      }),
    );
    const user = userEvent.setup();
    render(<LoginPage onLoggedIn={() => {}} />);

    await submitPhone(user, "+15551234567");
    await user.type(screen.getByLabelText(/code/i), "123456");
    const verifyButton = screen.getByRole("button", { name: /verify/i });
    await user.dblClick(verifyButton);

    expect(verifyOtp).toHaveBeenCalledOnce();
    resolveVerify();
  });
});
