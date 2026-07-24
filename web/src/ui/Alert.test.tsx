import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { Alert } from "./Alert";

// Step 3 (plan.md): shared Alert primitive — role="alert" display used by
// LoginPage, OtpPage, and NewJobPage's validation/server errors (criterion 3).

describe("Alert", () => {
  it("renders children with role=alert", () => {
    render(<Alert>Something went wrong</Alert>);

    expect(screen.getByRole("alert")).toHaveTextContent("Something went wrong");
  });

  it("renders nothing (not an empty element) when children is null", () => {
    const { container } = render(<Alert>{null}</Alert>);

    expect(container).toBeEmptyDOMElement();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});
