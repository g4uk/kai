import "@testing-library/jest-dom/vitest";
import { afterEach } from "vitest";
import { cleanup } from "@testing-library/react";

// `vitest.config.ts` sets `globals: false`, so Testing Library's built-in
// auto-cleanup (which only registers itself when a *global* `afterEach`
// exists) never runs. Register it explicitly so DOM trees from one test
// don't leak into the next.
afterEach(() => {
  cleanup();
});
