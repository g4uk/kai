import { useCallback, useRef, useState } from "react";
import { requestOtp, verifyOtp } from "../../api/client";

export type OtpFlowStep = "phone" | "otp" | "submitting";

export interface UseOtpFlowResult {
  step: OtpFlowStep;
  error: string | null;
  submitPhone: (phone: string) => Promise<void>;
  submitOtp: (code: string) => Promise<void>;
}

function messageOf(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return String(err);
}

/**
 * Owns the two-step login state machine (phone -> otp -> submitting) and is
 * the only piece that calls requestOtp/verifyOtp (specs/ui/plan.md step 3).
 */
export function useOtpFlow(onLoggedIn: () => void): UseOtpFlowResult {
  const [step, setStep] = useState<OtpFlowStep>("phone");
  const [error, setError] = useState<string | null>(null);
  const phoneRef = useRef<string>("");
  const submittingOtp = useRef(false);

  const submitPhone = useCallback(async (phone: string) => {
    phoneRef.current = phone;
    setError(null);
    try {
      await requestOtp(phone);
      setStep("otp");
    } catch (err) {
      setError(messageOf(err));
    }
  }, []);

  const submitOtp = useCallback(
    async (code: string) => {
      if (submittingOtp.current) {
        return;
      }
      submittingOtp.current = true;
      setError(null);
      setStep("submitting");
      try {
        await verifyOtp(phoneRef.current, code);
        onLoggedIn();
      } catch (err) {
        setError(messageOf(err));
        setStep("otp");
      } finally {
        submittingOtp.current = false;
      }
    },
    [onLoggedIn],
  );

  return { step, error, submitPhone, submitOtp };
}
