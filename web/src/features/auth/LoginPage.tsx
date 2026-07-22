import { useState, type FormEvent } from "react";
import { useOtpFlow } from "./useOtpFlow";
import { OtpPage } from "./OtpPage";

export interface LoginPageProps {
  onLoggedIn: () => void;
}

// Rough E.164 shape check: leading "+" then 1-15 digits. Mirrors the
// backend's e164Pattern closely enough to avoid an obviously-doomed
// request without re-implementing its exact regex (specs/ui/plan.md step 3).
const E164_PATTERN = /^\+[1-9]\d{1,14}$/;

/** Login screen: phone entry, then OTP entry (specs/ui/plan.md step 3). */
export function LoginPage({ onLoggedIn }: LoginPageProps) {
  const { step, error, submitPhone, submitOtp } = useOtpFlow(onLoggedIn);
  const [phone, setPhone] = useState("");
  const [validationError, setValidationError] = useState<string | null>(null);
  // Guards against a rapid double-click firing requestOtp twice, mirroring
  // the isSubmitting guard the OTP step already uses (reviewer nit fix).
  const [isSubmittingPhone, setIsSubmittingPhone] = useState(false);

  function handlePhoneSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isSubmittingPhone) {
      return;
    }
    if (!E164_PATTERN.test(phone)) {
      setValidationError("Enter a valid phone number (e.g. +15551234567).");
      return;
    }
    setValidationError(null);
    setIsSubmittingPhone(true);
    void submitPhone(phone).finally(() => setIsSubmittingPhone(false));
  }

  if (step === "otp" || step === "submitting") {
    return (
      <OtpPage
        phone={phone}
        error={error}
        isSubmitting={step === "submitting"}
        onSubmit={(code) => void submitOtp(code)}
      />
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <form onSubmit={handlePhoneSubmit} className="flex flex-col gap-4">
        <label className="flex flex-col gap-1">
          <span>Phone number</span>
          <input
            type="tel"
            value={phone}
            onChange={(event) => setPhone(event.target.value)}
            disabled={isSubmittingPhone}
          />
        </label>
        {validationError !== null && <p role="alert">{validationError}</p>}
        <button type="submit" disabled={isSubmittingPhone}>
          Send code
        </button>
      </form>
    </div>
  );
}
