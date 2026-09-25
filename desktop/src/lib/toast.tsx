import { CircleAlert, CircleCheck, TriangleAlert } from "lucide-react";
import { createContext, useCallback, useContext, useState, type ReactNode } from "react";

interface Toast {
  id: number;
  message: string;
  variant: "error" | "warning" | "success";
}

interface ToastContextValue {
  notify: (message: string, variant?: Toast["variant"]) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

let nextId = 0;

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const notify = useCallback((message: string, variant: Toast["variant"] = "error") => {
    const id = nextId++;
    setToasts((current) => [...current, { id, message, variant }]);
    setTimeout(() => {
      setToasts((current) => current.filter((t) => t.id !== id));
    }, 5000);
  }, []);

  return (
    <ToastContext.Provider value={{ notify }}>
      {children}
      {/* A notification, not a banner: a raised surface in the theme's own
          colours, with the tone carried by its icon. */}
      <div className="fixed right-4 bottom-4 z-50 flex w-80 flex-col gap-2" role="status" aria-live="polite">
        {toasts.map((t) => (
          <div
            key={t.id}
            className="flex items-start gap-2.5 rounded-lg bg-popover px-3.5 py-3 text-[13px] text-popover-foreground shadow-lg ring-1 ring-border select-text"
          >
            <ToastIcon variant={t.variant} />
            <span className="min-w-0 break-words">{t.message}</span>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
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
