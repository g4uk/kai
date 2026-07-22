import { BrowserRouter } from "react-router-dom";
import { AuthProvider } from "./app/AuthContext";
import { AppRoutes } from "./app/router";

/**
 * App shell entry point (specs/ui/plan.md step 4): composes the router,
 * AuthContext, and the real route tree. This is what main.tsx mounts — the
 * step-1 placeholder previously left here never wired these together, so a
 * real page load rendered only the static heading with no routes/login.
 */
function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <AppRoutes />
      </AuthProvider>
    </BrowserRouter>
  );
}

export default App;
