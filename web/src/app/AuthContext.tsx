import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useNavigate } from "react-router-dom";
import {
  getAuthMe,
  logout as apiLogout,
  setOnUnauthorized,
} from "../api/client";

export type AuthStatus = "unknown" | "authenticated" | "anonymous";

interface AuthContextValue {
  status: AuthStatus;
  /** Probes GET /auth/me once, if status is still "unknown" (called by ProtectedRoute on mount). */
  ensureChecked: () => void;
  /** Marks the session as authenticated without re-probing (used right after OTP verify). */
  setAuthenticated: () => void;
  /** Calls POST /auth/logout (best-effort), then resets to "unknown" and navigates to /login. */
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

/**
 * Holds session-auth state (specs/ui/plan.md step 4). Auth is detected by
 * probing `GET /auth/me` (specs/auth-me/spec.md): success (204) means
 * authenticated, any error (including 401) means anonymous. Also registers
 * itself as the API client's global 401 handler so any call, anywhere, that
 * gets a 401 clears state and lets ProtectedRoute redirect.
 */
export function AuthProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<AuthStatus>("unknown");
  const navigate = useNavigate();
  const checking = useRef(false);

  useEffect(() => {
    setOnUnauthorized(() => {
      setStatus("anonymous");
    });
  }, []);

  const ensureChecked = useCallback(() => {
    if (status !== "unknown" || checking.current) {
      return;
    }
    checking.current = true;
    getAuthMe()
      .then(() => setStatus("authenticated"))
      .catch(() => setStatus("anonymous"))
      .finally(() => {
        checking.current = false;
      });
  }, [status]);

  const setAuthenticated = useCallback(() => {
    setStatus("authenticated");
  }, []);

  const logout = useCallback(async () => {
    try {
      await apiLogout();
    } catch {
      // Logout should never strand a user on an authenticated-looking
      // screen, even if the network call itself fails.
    }
    setStatus("unknown");
    // Deliberately a push, not a replace: replacing here would overwrite the
    // current protected route's history entry, so a subsequent browser
    // back-navigation would skip right past it (criterion 16 needs back-nav
    // to land back on the protected route and re-probe, not skip it).
    navigate("/login");
  }, [navigate]);

  return (
    <AuthContext.Provider
      value={{ status, ensureChecked, setAuthenticated, logout }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthContextValue {
  const value = useContext(AuthContext);
  if (value === null) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return value;
}

/** Logs out and redirects to /login regardless of whether the call succeeds. */
export function LogoutButton() {
  const { logout } = useAuth();
  return (
    <button
      type="button"
      onClick={() => {
        void logout();
      }}
      className="text-sm text-blue-600 underline hover:text-blue-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 rounded-sm"
    >
      Log out
    </button>
  );
}
