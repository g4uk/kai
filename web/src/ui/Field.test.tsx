import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Field } from "./Field";

// Step 2 (plan.md): shared Field primitive (labeled single-line text input)
// — used for phone number, OTP code, and YouTube URL entry (criteria 1, 2, 6).

describe("Field", () => {
  it("associates the label with the input so getByLabelText finds it", () => {
    render(<Field label="Phone number" value="" onChange={() => {}} />);

    expect(screen.getByLabelText("Phone number")).toBeInTheDocument();
  });

  it("calls onChange while typing", async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();
    render(<Field label="Code" value="" onChange={onChange} />);

    await user.type(screen.getByLabelText("Code"), "1");

    expect(onChange).toHaveBeenCalled();
  });

  it("forwards disabled", () => {
    render(<Field label="Code" value="" onChange={() => {}} disabled />);

    expect(screen.getByLabelText("Code")).toBeDisabled();
  });

  it("bakes in a focus-ring class on the input (criterion 1)", () => {
    render(<Field label="Code" value="" onChange={() => {}} />);

    expect(screen.getByLabelText("Code").className).toContain(
      "focus-visible:ring",
    );
  });
});
