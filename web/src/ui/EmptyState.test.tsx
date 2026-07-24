import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { EmptyState } from "./EmptyState";

// Step 6 (plan.md): shared EmptyState primitive — used today only by
// JobListPage's zero-jobs state (criterion 10).

describe("EmptyState", () => {
  it("renders heading and body text", () => {
    render(
      <MemoryRouter>
        <EmptyState
          heading="No analyses yet"
          body="You have not submitted any analyses yet."
        />
      </MemoryRouter>,
    );

    expect(screen.getByText("No analyses yet")).toBeInTheDocument();
    expect(
      screen.getByText("You have not submitted any analyses yet."),
    ).toBeInTheDocument();
  });

  it("renders a link with the given accessible name when a link target/label is given", () => {
    render(
      <MemoryRouter>
        <EmptyState
          heading="No analyses yet"
          body="You have not submitted any analyses yet."
          linkTo="/jobs/new"
          linkLabel="Submit your first analysis"
        />
      </MemoryRouter>,
    );

    expect(
      screen.getByRole("link", { name: "Submit your first analysis" }),
    ).toBeInTheDocument();
  });

  it("renders a visible focus ring on the link (criterion 1)", () => {
    render(
      <MemoryRouter>
        <EmptyState
          heading="No analyses yet"
          body="You have not submitted any analyses yet."
          linkTo="/jobs/new"
          linkLabel="Submit your first analysis"
        />
      </MemoryRouter>,
    );

    const link = screen.getByRole("link", {
      name: "Submit your first analysis",
    });
    expect(link.className).toContain("focus-visible:ring");
  });
});
