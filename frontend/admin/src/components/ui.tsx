import type {
  ButtonHTMLAttributes,
  InputHTMLAttributes,
  ReactNode,
  SelectHTMLAttributes,
} from "react";

export function cn(...parts: Array<string | false | undefined>): string {
  return parts.filter(Boolean).join(" ");
}

type CardProps = {
  title: string;
  subtitle?: string;
  children: ReactNode;
  className?: string;
  actions?: ReactNode;
};

export function Card({ title, subtitle, children, className, actions }: CardProps) {
  return (
    <section
      className={cn(
        "rounded-xl border border-slate-800/90 bg-slate-900/50 p-4 shadow-sm shadow-black/20",
        className
      )}
    >
      <div className="mb-3 flex items-start justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold tracking-tight text-slate-100">{title}</h2>
          {subtitle ? <p className="mt-0.5 text-xs text-slate-500">{subtitle}</p> : null}
        </div>
        {actions}
      </div>
      {children}
    </section>
  );
}

type BadgeVariant = "ok" | "warn" | "err" | "neutral" | "info";

const badgeStyles: Record<BadgeVariant, string> = {
  ok: "bg-emerald-500/15 text-emerald-300 ring-emerald-500/30",
  warn: "bg-amber-500/15 text-amber-300 ring-amber-500/30",
  err: "bg-rose-500/15 text-rose-300 ring-rose-500/30",
  neutral: "bg-slate-500/15 text-slate-300 ring-slate-500/30",
  info: "bg-sky-500/15 text-sky-300 ring-sky-500/30",
};

export function Badge({
  variant = "neutral",
  children,
  className,
}: {
  variant?: BadgeVariant;
  children: ReactNode;
  className?: string;
}) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-medium ring-1 ring-inset",
        badgeStyles[variant],
        className
      )}
    >
      {children}
    </span>
  );
}

type BtnVariant = "primary" | "secondary" | "ghost" | "danger";

const btnVariant: Record<BtnVariant, string> = {
  primary: "bg-emerald-600 text-white hover:bg-emerald-500 focus-visible:ring-emerald-500",
  secondary: "bg-slate-700 text-slate-100 hover:bg-slate-600 focus-visible:ring-slate-500",
  ghost: "bg-transparent text-slate-300 hover:bg-slate-800 focus-visible:ring-slate-500",
  danger: "bg-rose-600/90 text-white hover:bg-rose-500 focus-visible:ring-rose-500",
};

export function Button({
  variant = "secondary",
  className,
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: BtnVariant }) {
  return (
    <button
      type="button"
      className={cn(
        "inline-flex items-center justify-center rounded-lg px-2.5 py-1.5 text-xs font-medium transition",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:ring-offset-slate-950",
        "disabled:cursor-not-allowed disabled:opacity-40",
        btnVariant[variant],
        className
      )}
      {...props}
    />
  );
}

export function FieldLabel({ children, htmlFor }: { children: ReactNode; htmlFor?: string }) {
  return (
    <label htmlFor={htmlFor} className="text-xs font-medium text-slate-400">
      {children}
    </label>
  );
}

const fieldClass =
  "w-full rounded-lg border border-slate-700 bg-slate-950/80 px-2.5 py-1.5 text-sm text-slate-100 placeholder:text-slate-600 focus:border-emerald-500/60 focus:outline-none focus:ring-2 focus:ring-emerald-500/30";

export function Input({ className, ...props }: InputHTMLAttributes<HTMLInputElement>) {
  return <input className={cn(fieldClass, className)} {...props} />;
}

export function Select({ className, children, ...props }: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select className={cn(fieldClass, "pr-8", className)} {...props}>
      {children}
    </select>
  );
}

export function ActionRow({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn("flex flex-wrap items-center gap-2", className)}>{children}</div>;
}

export function Divider() {
  return <div className="my-3 border-t border-slate-800" />;
}

export function EmptyState({ children }: { children: ReactNode }) {
  return (
    <p className="rounded-lg border border-dashed border-slate-800 bg-slate-950/40 px-3 py-6 text-center text-xs text-slate-500">
      {children}
    </p>
  );
}
