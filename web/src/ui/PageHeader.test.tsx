import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { PageHeader } from "./PageHeader";

// Step 5 (plan.md): shared PageHeader primitive — title + a slot for
// LogoutButton, used by JobListPage, NewJobPage, and JobResultsPage
// (criterion 7).

describe("PageHeader", () => {
  it("renders the given title as a heading", () => {
    render(<PageHeader title="Jobs" />);

    expect(screen.getByRole("heading", { name: "Jobs" })).toBeInTheDocument();
  });

  it("renders passed-in children (e.g. a stub logout button) alongside the title", () => {
    render(
      <PageHeader title="Jobs">
        <button>Log out</button>
      </PageHeader>,
    );

    expect(screen.getByRole("heading", { name: "Jobs" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Log out" })).toBeInTheDocument();
  });
});
