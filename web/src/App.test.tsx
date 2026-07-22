import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import App from "./App";

// Step 1 (plan.md): scaffold smoke test — proves the Vite + React + TS +
// Vitest + Tailwind toolchain works end-to-end, before any routes/features
// exist. `text-xl` is the cheapest signal that a Tailwind utility class made
// it into rendered markup (jsdom cannot compute actual visual style).
describe("App", () => {
  it("renders the placeholder heading with a Tailwind utility class", () => {
    render(<App />);

    const heading = screen.getByText("Kumite Analyzer");
    expect(heading).toBeInTheDocument();
    expect(heading).toHaveClass("text-xl");
  });
});
