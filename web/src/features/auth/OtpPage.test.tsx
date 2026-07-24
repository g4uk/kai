import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { OtpPage } from "./OtpPage";

// Step 3 (plan.md): OtpPage — presentational OTP-code entry screen. Tested
// standalone (props-driven), independent of useOtpFlow's wiring (covered by
// useOtpFlow.test.ts and LoginPage.test.tsx).

describe("OtpPage", () => {
  it("renders a code input and submits the entered code", async () => {
    const onSubmit = vi.fn();
    const user = userEvent.setup();
    render(
      <OtpPage
        phone="+15551234567"
        error={null}
        isSubmitting={false}
        onSubmit={onSubmit}
      />,
    );

    await user.type(screen.getByLabelText(/code/i), "123456");
    await user.click(screen.getByRole("button", { name: /verify/i }));

    expect(onSubmit).toHaveBeenCalledExactlyOnceWith("123456");
  });

  it("displays the inline error text passed in and keeps the form visible (criterion 4)", () => {
    render(
      <OtpPage
        phone="+15551234567"
        error="invalid or expired code"
        isSubmitting={false}
        onSubmit={() => {}}
      />,
    );

    expect(screen.getByText("invalid or expired code")).toBeInTheDocument();
    expect(screen.getByLabelText(/code/i)).toBeInTheDocument();
  });

  it("renders no error text when error is null", () => {
    render(
      <OtpPage
        phone="+15551234567"
        error={null}
        isSubmitting={false}
        onSubmit={() => {}}
      />,
    );

    expect(screen.queryByText(/invalid/i)).not.toBeInTheDocument();
  });

  it("disables the submit control while isSubmitting is true, guarding double-submit (edge case 7)", async () => {
    const onSubmit = vi.fn();
    const user = userEvent.setup();
    render(
      <OtpPage
        phone="+15551234567"
        error={null}
        isSubmitting={true}
        onSubmit={onSubmit}
      />,
    );

    const verifyButton = screen.getByRole("button", { name: /verify/i });
    expect(verifyButton).toBeDisabled();

    await user.click(verifyButton);
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("renders inside a centered, width-constrained card layout (criterion 8, plan step 7)", () => {
    const { container } = render(
      <OtpPage
        phone="+15551234567"
        error={null}
        isSubmitting={false}
        onSubmit={() => {}}
      />,
    );

    expect(container.querySelector(".max-w-sm")).not.toBeNull();
  });
});
