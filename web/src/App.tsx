import { BrowserRouter } from "react-router-dom";
import { AuthProvider } from "./app/AuthContext";
import { AppRoutes } from "./app/router";
import { ToastProvider } from "./ui/Toast";
import { useJobStatusEvents } from "./features/notifications/useJobStatusEvents";

/** Mounted once inside AuthProvider/ToastProvider — wires the SSE hook into the app shell, renders nothing (specs/popup-notifications+sse/plan.md step 7). */
function JobStatusNotifier() {
  useJobStatusEvents();
  return null;
}

/**
 * App shell entry point (specs/ui/plan.md step 4): composes the router,
 * AuthContext, and the real route tree. This is what main.tsx mounts — the
 * step-1 placeholder previously left here never wired these together, so a
 * real page load rendered only the static heading with no routes/login.
 *
 * ToastProvider + JobStatusNotifier (specs/popup-notifications+sse/plan.md
 * step 7) are nested inside AuthProvider so useAuth()/useToast() are both
 * available to the SSE hook, and mounted once at the app root so a popup can
 * appear regardless of which screen the user is currently on.
 */
function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <ToastProvider>
          <JobStatusNotifier />
          <AppRoutes />
        </ToastProvider>
      </AuthProvider>
    </BrowserRouter>
  );
}

export default App;
