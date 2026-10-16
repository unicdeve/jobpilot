import { CircleAlert, CircleCheck, TriangleAlert } from "lucide-react";
import { createContext, type ReactNode, useCallback, useContext, useState } from "react";
import { createPortal } from "react-dom";
import { errorMessage } from "./api";

interface Toast {
  id: number;
  message: string;
  variant: "error" | "warning" | "success";
}

interface ToastContextValue {
  notify: (message: string, variant?: Toast["variant"]) => void;
  /// notifyError says what went wrong, for a failed request or mutation.
  notifyError: (err: unknown) => void;
  /// activity is where work still under way is shown, above the toasts.
  activity: HTMLElement | null;
}

const ToastContext = createContext<ToastContextValue | null>(null);

let nextId = 0;

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const [activity, setActivity] = useState<HTMLElement | null>(null);

  const notify = useCallback((message: string, variant: Toast["variant"] = "error") => {
    const id = nextId++;
    setToasts((current) => [...current, { id, message, variant }]);
    setTimeout(() => {
      setToasts((current) => current.filter((t) => t.id !== id));
    }, 5000);
  }, []);
  const notifyError = useCallback((err: unknown) => notify(errorMessage(err)), [notify]);

  return (
    <ToastContext.Provider value={{ notify, notifyError, activity }}>
      {children}
      {/* A notification, not a banner: a raised surface in the theme's own
          colours, with the tone carried by its icon. */}
      <div className="fixed right-4 bottom-4 z-50 flex w-80 flex-col gap-2" role="status" aria-live="polite">
        <div ref={setActivity} className="flex flex-col gap-2 empty:hidden" />
        {toasts.map((t) => (
          <div key={t.id} className={SURFACE}>
            <ToastIcon variant={t.variant} />
            <span className="min-w-0 wrap-break-word">{t.message}</span>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

/// SURFACE is how a toast and an activity both look: one stack, one kind
/// of thing in it.
const SURFACE =
  "flex items-start gap-2.5 rounded-lg bg-popover px-3.5 py-3 text-[13px] text-popover-foreground shadow-lg ring-1 ring-border select-text";

/// ToastActivity shows work still under way in the toast stack, for as long
/// as it is rendered. A toast says something happened and then goes; this
/// stays until whatever it describes has finished.
export function ToastActivity({ children }: { children: ReactNode }) {
  const { activity } = useToast();
  return activity ? createPortal(<div className={SURFACE}>{children}</div>, activity) : null;
}

function ToastIcon({ variant }: { variant: Toast["variant"] }) {
  switch (variant) {
    case "error":
      return <CircleAlert className="mt-px size-4 shrink-0 text-destructive" />;
    case "warning":
      return <TriangleAlert className="mt-px size-4 shrink-0 text-warning" />;
    case "success":
      return <CircleCheck className="mt-px size-4 shrink-0 text-success" />;
  }
}

export function useToast(): ToastContextValue {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error("useToast must be used within a ToastProvider");
  return ctx;
}
