import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { StatusMessage } from "./StatusMessage";

// Step 6 (plan.md): shared StatusMessage primitive — one consistent
// container/text style for "Loading…" (JobListPage) and "Analysis in
// progress…" (JobResultsPage), criterion 9. No built-in text of its own.

describe("StatusMessage", () => {
  it("renders its children inside the shared container", () => {
    render(<StatusMessage>Loading…</StatusMessage>);

    expect(screen.getByText("Loading…")).toBeInTheDocument();
  });

  it("renders whatever children it is given, with no built-in text of its own", () => {
    render(<StatusMessage>Analysis in progress…</StatusMessage>);

    expect(screen.getByText("Analysis in progress…")).toBeInTheDocument();
    expect(screen.queryByText("Loading…")).not.toBeInTheDocument();
  });
});
