import { describe, it, expect, vi, afterEach } from "vitest";
import {
  render,
  screen,
  fireEvent,
  renderHook,
  act,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ToastProvider, useToast } from "./Toast";

// Step 6 (plan.md): new toast/popup UI primitive — ToastProvider/useToast,
// following the file-per-primitive pattern in web/src/ui/ (Alert, EmptyState).
//
// Auto-dismiss duration: the plan says "a fixed duration, e.g. 5s" without
// pinning an exact number. This test file is the source of truth for that
// choice — 5000ms — and the implementer must match it exactly, or update
// this constant in lockstep.
const AUTO_DISMISS_MS = 5000;

function AddToastButton({ message }: { message: string }) {
  const { addToast } = useToast();
  return (
    <button type="button" onClick={() => addToast(message)}>
      add-toast
    </button>
  );
}

describe("Toast", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("renders an added toast's message", async () => {
    const user = userEvent.setup();
    render(
      <ToastProvider>
        <AddToastButton message="Job done" />
      </ToastProvider>,
    );

    await user.click(screen.getByRole("button", { name: "add-toast" }));

    expect(screen.getByText("Job done")).toBeInTheDocument();
  });

  it(`auto-dismisses a toast after ${AUTO_DISMISS_MS}ms`, () => {
    vi.useFakeTimers();
    render(
      <ToastProvider>
        <AddToastButton message="Job done" />
      </ToastProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "add-toast" }));
    expect(screen.getByText("Job done")).toBeInTheDocument();

    act(() => {
      vi.advanceTimersByTime(AUTO_DISMISS_MS);
    });

    expect(screen.queryByText("Job done")).not.toBeInTheDocument();
  });

  it("removes a toast immediately when its dismiss button is clicked, before the auto-dismiss timer fires", async () => {
    const user = userEvent.setup();
    render(
      <ToastProvider>
        <AddToastButton message="Job done" />
      </ToastProvider>,
    );

    await user.click(screen.getByRole("button", { name: "add-toast" }));
    expect(screen.getByText("Job done")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /dismiss/i }));

    expect(screen.queryByText("Job done")).not.toBeInTheDocument();
  });

  it("renders two simultaneously-added toasts", async () => {
    const user = userEvent.setup();
    render(
      <ToastProvider>
        <AddToastButton message="Job 1 done" />
        <AddToastButton message="Job 2 failed" />
      </ToastProvider>,
    );

    // Both buttons share the accessible name "add-toast" from the moment
    // they mount, so getByRole would throw; grab all and click by index.
    const addButtons = screen.getAllByRole("button", { name: "add-toast" });
    await user.click(addButtons[0]);
    await user.click(addButtons[1]);

    expect(screen.getByText("Job 1 done")).toBeInTheDocument();
    expect(screen.getByText("Job 2 failed")).toBeInTheDocument();
  });

  it("useToast throws when called outside a ToastProvider (mirrors useAuth's guard)", () => {
    const consoleError = vi
      .spyOn(console, "error")
      .mockImplementation(() => {});

    expect(() => renderHook(() => useToast())).toThrow(
      /useToast must be used within a ToastProvider/i,
    );

    consoleError.mockRestore();
  });
});
