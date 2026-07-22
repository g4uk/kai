import { describe, it, expect, vi, beforeEach } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { useOtpFlow } from "./useOtpFlow";

// Step 3 (plan.md): useOtpFlow owns the two-step login state machine and is
// the only piece that calls requestOtp/verifyOtp — kept out of the
// components per frontend skill rule 6 ("a hook with logic = its own test
// via renderHook"). requestOtp/verifyOtp are faked at the API-client-module
// boundary (frontend skill rule 3: fake the interface, not raw fetch, since
// client.test.ts already covers the raw-fetch boundary).

const { requestOtp, verifyOtp } = vi.hoisted(() => ({
  requestOtp: vi.fn(),
  verifyOtp: vi.fn(),
}));

vi.mock("../../api/client", () => ({
  requestOtp,
  verifyOtp,
}));

/** A promise the test controls the resolution/rejection timing of. */
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe("useOtpFlow", () => {
  beforeEach(() => {
    requestOtp.mockReset();
    verifyOtp.mockReset();
  });

  it("starts on the phone step with no error", () => {
    const { result } = renderHook(() => useOtpFlow(() => {}));

    expect(result.current.step).toBe("phone");
    expect(result.current.error).toBeNull();
  });

  it("submitting the phone calls requestOtp and transitions to the otp step on success", async () => {
    requestOtp.mockResolvedValue(undefined);
    const { result } = renderHook(() => useOtpFlow(() => {}));

    await act(async () => {
      await result.current.submitPhone("+15551234567");
    });

    expect(requestOtp).toHaveBeenCalledExactlyOnceWith("+15551234567");
    expect(result.current.step).toBe("otp");
    expect(result.current.error).toBeNull();
  });

  it("a rejected phone submission surfaces the error and stays off the otp step", async () => {
    requestOtp.mockRejectedValue(new Error("invalid phone_number"));
    const { result } = renderHook(() => useOtpFlow(() => {}));

    await act(async () => {
      await result.current.submitPhone("+1garbage");
    });

    expect(result.current.step).not.toBe("otp");
    expect(result.current.error).toBe("invalid phone_number");
  });

  it("a rejected otp submission surfaces the error and stays on the otp step", async () => {
    requestOtp.mockResolvedValue(undefined);
    verifyOtp.mockRejectedValue(new Error("invalid or expired code"));
    const { result } = renderHook(() => useOtpFlow(() => {}));

    await act(async () => {
      await result.current.submitPhone("+15551234567");
    });
    await act(async () => {
      await result.current.submitOtp("000000");
    });

    expect(result.current.step).toBe("otp");
    expect(result.current.error).toBe("invalid or expired code");
  });

  it("a successful otp submission calls onLoggedIn", async () => {
    requestOtp.mockResolvedValue(undefined);
    verifyOtp.mockResolvedValue(undefined);
    const onLoggedIn = vi.fn();
    const { result } = renderHook(() => useOtpFlow(onLoggedIn));

    await act(async () => {
      await result.current.submitPhone("+15551234567");
    });
    await act(async () => {
      await result.current.submitOtp("123456");
    });

    expect(onLoggedIn).toHaveBeenCalledOnce();
  });

  it("transiently reports a submitting step while an otp verification is in flight", async () => {
    requestOtp.mockResolvedValue(undefined);
    const gate = deferred<void>();
    verifyOtp.mockReturnValue(gate.promise);
    const { result } = renderHook(() => useOtpFlow(() => {}));

    await act(async () => {
      await result.current.submitPhone("+15551234567");
    });

    act(() => {
      void result.current.submitOtp("123456");
    });

    await waitFor(() => expect(result.current.step).toBe("submitting"));

    await act(async () => {
      gate.resolve(undefined);
      await gate.promise;
    });
  });

  it("a second otp submission while one is pending is a no-op (no second verifyOtp call)", async () => {
    requestOtp.mockResolvedValue(undefined);
    const gate = deferred<void>();
    verifyOtp.mockReturnValue(gate.promise);
    const { result } = renderHook(() => useOtpFlow(() => {}));

    await act(async () => {
      await result.current.submitPhone("+15551234567");
    });

    act(() => {
      void result.current.submitOtp("123456");
      void result.current.submitOtp("123456");
    });

    expect(verifyOtp).toHaveBeenCalledOnce();

    await act(async () => {
      gate.resolve(undefined);
      await gate.promise;
    });
  });
});
