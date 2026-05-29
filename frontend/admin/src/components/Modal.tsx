import type { ReactNode } from "react";
import { Button, cn } from "./ui.js";

type Props = {
  open: boolean;
  title: string;
  onClose: () => void;
  children: ReactNode;
  footer?: ReactNode;
  className?: string;
  wide?: boolean;
};

export function Modal({ open, title, onClose, children, footer, className, wide }: Props) {
  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4">
      <div
        className={cn(
          "flex max-h-[90vh] w-full flex-col rounded-xl border border-slate-800 bg-slate-900 shadow-xl",
          wide ? "max-w-3xl" : "max-w-lg",
          className
        )}
        role="dialog"
        aria-modal="true"
        aria-labelledby="modal-title"
      >
        <div className="flex items-center justify-between border-b border-slate-800 px-4 py-3">
          <h3 id="modal-title" className="text-base font-semibold text-slate-100">
            {title}
          </h3>
          <Button variant="ghost" onClick={onClose} aria-label="Close">
            ×
          </Button>
        </div>
        <div className="overflow-y-auto px-4 py-3 scrollbar-thin">{children}</div>
        {footer ? (
          <div className="flex justify-end gap-2 border-t border-slate-800 px-4 py-3">{footer}</div>
        ) : null}
      </div>
    </div>
  );
}
