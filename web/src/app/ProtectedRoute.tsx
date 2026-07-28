import { useEffect, type ReactNode } from "react";
import { Navigate, useLocation } from "react-router-dom";
import { useAuth } from "./AuthContext";

/**
 * Renders `children` only once the session is confirmed authenticated.
 * While "unknown", triggers a probe and renders nothing; renders a
 * redirect to /login once "anonymous" (specs/ui/plan.md step 4). Also
 * re-probes on every navigation into a protected route, even when React
 * Router reuses this component instance across sibling routes (e.g.
 * /jobs -> /jobs/123) instead of unmounting/remounting it — the `pathname`
 * dependency below is what drives that re-probe (already-authenticated
 * content still renders optimistically while it runs in the background;
 * specs/session-revalidation/spec.md).
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
