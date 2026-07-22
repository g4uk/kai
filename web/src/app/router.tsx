import { Navigate, Route, Routes, useNavigate } from "react-router-dom";
import { useAuth } from "./AuthContext";
import { ProtectedRoute } from "./ProtectedRoute";
import { LoginPage } from "../features/auth/LoginPage";
import { JobListPage } from "../features/jobs/JobListPage";
import { NewJobPage } from "../features/jobs/NewJobPage";
import { JobResultsPage } from "../features/jobs/JobResultsPage";

function LoginRoute() {
  const { status, setAuthenticated } = useAuth();
  const navigate = useNavigate();

  if (status === "authenticated") {
    return <Navigate to="/jobs" replace />;
  }

  return (
    <LoginPage
      onLoggedIn={() => {
        setAuthenticated();
        navigate("/jobs", { replace: true });
      }}
    />
  );
}

/** All app routes (specs/ui/plan.md step 4). Mounted under a Router by the caller. */
export function AppRoutes() {
  return (
    <Routes>
      <Route path="/login" element={<LoginRoute />} />
      <Route path="/" element={<Navigate to="/jobs" replace />} />
      <Route
        path="/jobs"
        element={
          <ProtectedRoute>
            <JobListPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/jobs/new"
        element={
          <ProtectedRoute>
            <NewJobPage />
          </ProtectedRoute>
        }
      />
      <Route
        path="/jobs/:id"
        element={
          <ProtectedRoute>
            <JobResultsPage />
          </ProtectedRoute>
        }
      />
    </Routes>
  );
}
