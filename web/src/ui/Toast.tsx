import {
  createContext,
  useCallback,
  useContext,
  useRef,
  useState,
  type ReactNode,
} from "react";

/** Auto-dismiss duration for a toast, in milliseconds (Toast.test.tsx is the source of truth). */
const AUTO_DISMISS_MS = 5000;

interface ToastItem {
  id: number;
  message: string;
}

interface ToastContextValue {
  addToast: (message: string) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

/**
 * Holds a fixed-position stack of dismissible, auto-timing-out popup
 * notifications (specs/popup-notifications+sse/plan.md step 6), following
 * the file-per-primitive pattern established by Alert.tsx/EmptyState.tsx.
 */
export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const nextId = useRef(0);

  const removeToast = useCallback((id: number) => {
    setToasts((current) => current.filter((toast) => toast.id !== id));
  }, []);

  const addToast = useCallback(
    (message: string) => {
      const id = nextId.current++;
      setToasts((current) => [...current, { id, message }]);
      setTimeout(() => {
        removeToast(id);
      }, AUTO_DISMISS_MS);
    },
    [removeToast],
  );

  return (
    <ToastContext.Provider value={{ addToast }}>
      {children}
      <div className="fixed bottom-4 right-4 z-50 flex flex-col gap-2">
        {toasts.map((toast) => (
          <div
            key={toast.id}
            role="status"
            className="flex items-start gap-3 rounded-md border border-slate-200 bg-white px-4 py-3 text-sm text-slate-900 shadow-lg"
          >
            <span className="flex-1">{toast.message}</span>
            <button
              type="button"
              aria-label="Dismiss"
              onClick={() => removeToast(toast.id)}
              className="text-slate-500 hover:text-slate-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 rounded-sm"
            >
              &times;
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast(): ToastContextValue {
  const value = useContext(ToastContext);
  if (value === null) {
    throw new Error("useToast must be used within a ToastProvider");
  }
  return value;
}
