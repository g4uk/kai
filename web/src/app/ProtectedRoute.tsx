import { useEffect, type ReactNode } from "react";
import { Navigate, useLocation } from "react-router-dom";
import { useAuth } from "./AuthContext";

/**
 * Renders `children` only once the session is confirmed authenticated.
 * While "unknown", triggers a probe and renders nothing; renders a
 * redirect to /login once "anonymous" (specs/ui/plan.md step 4).
 */
export function ProtectedRoute({ children }: { children: ReactNode }) {
  const { status, ensureChecked } = useAuth();
  const { pathname } = useLocation();

  useEffect(() => {
    ensureChecked();
  }, [ensureChecked, pathname]);

  if (status === "unknown") {
    return null;
  }

  if (status === "anonymous") {
    return <Navigate to="/login" replace />;
  }

  return <>{children}</>;
}
