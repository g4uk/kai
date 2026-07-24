import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Button } from "./Button";

// Step 1 (plan.md): shared Button primitive — used by every primary submit
// action across the five screens (criteria 1, 2, 5).

describe("Button", () => {
  it("renders as a real <button> with the given type and text", () => {
    render(<Button type="submit">Send code</Button>);

    const button = screen.getByRole("button", { name: "Send code" });
    expect(button.tagName).toBe("BUTTON");
    expect(button).toHaveAttribute("type", "submit");
  });

  it("forwards disabled", () => {
    render(
      <Button type="button" disabled>
        Submit
      </Button>,
    );

    expect(screen.getByRole("button", { name: "Submit" })).toBeDisabled();
  });

  it("forwards onClick", async () => {
    const onClick = vi.fn();
    const user = userEvent.setup();
    render(
      <Button type="button" onClick={onClick}>
        Click me
      </Button>,
    );

    await user.click(screen.getByRole("button", { name: "Click me" }));

    expect(onClick).toHaveBeenCalledOnce();
  });

  it("bakes in a focus-ring and disabled-cursor class (criteria 1, 2, 5)", () => {
    render(<Button type="button">Submit</Button>);

    const button = screen.getByRole("button", { name: "Submit" });
    expect(button.className).toContain("focus-visible:ring");
    expect(button.className).toContain("disabled:cursor-not-allowed");
  });
});
