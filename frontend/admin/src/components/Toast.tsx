import { createContext, useContext, useState, type ReactNode } from "react";
import { cn } from "./ui.js";

type ToastState = { message: string; variant: "ok" | "err" } | null;

type ToastContextValue = {
  showToast: (message: string, variant?: "ok" | "err") => void;
};

const ToastContext = createContext<ToastContextValue | null>(null);

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toast, setToast] = useState<ToastState>(null);

  function showToast(message: string, variant: "ok" | "err" = "ok") {
    setToast({ message, variant });
    setTimeout(() => setToast(null), 2000);
  }

  return (
    <ToastContext.Provider value={{ showToast }}>
      {children}
      {toast ? (
        <div
          className={cn(
            "fixed bottom-4 right-4 z-[100] rounded-lg px-4 py-2 text-sm shadow-lg",
            toast.variant === "ok"
              ? "bg-slate-800 text-emerald-200 ring-1 ring-emerald-500/30"
              : "bg-slate-800 text-rose-200 ring-1 ring-rose-500/30"
          )}
          role="status"
        >
          {toast.message}
        </div>
      ) : null}
    </ToastContext.Provider>
  );
}

export function useToast(): ToastContextValue {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error("useToast must be used within ToastProvider");
  return ctx;
}
