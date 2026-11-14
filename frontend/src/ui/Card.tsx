import type { ReactNode } from "react";

export function Card({ title, actions, children, className = "" }: { title?: ReactNode; actions?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section className={`card p-5 ${className}`}>
      {(title || actions) && (
        <header className="mb-4 flex items-center justify-between gap-3">
          {title && <h2 className="text-sm font-semibold text-slate-700">{title}</h2>}
          {actions}
        </header>
      )}
      {children}
    </section>
  );
}

export function Kpi({ label, value, hint, delta }: { label: string; value: ReactNode; hint?: ReactNode; delta?: ReactNode }) {
  return (
    <div className="card p-4">
      <p className="label">{label}</p>
      <p className="mt-1 flex items-baseline gap-2 text-2xl font-semibold text-slate-900">
        {value} {delta}
      </p>
      {hint && <p className="mt-1 text-xs text-slate-500">{hint}</p>}
    </div>
  );
}

/** Coloured delta. `good` says whether an increase is good (pass rate) or bad (tokens, latency). */
export function Delta({ value, text, good = "up" }: { value: number; text: string; good?: "up" | "down" }) {
  const better = good === "up" ? value > 0 : value < 0;
  const tone = value === 0 ? "text-slate-400" : better ? "text-emerald-600" : "text-rose-600";
  return <span className={`text-sm font-medium ${tone}`}>{text}</span>;
}

export function Chip({ children, tone = "bg-slate-100 text-slate-700" }: { children: ReactNode; tone?: string }) {
  return <span className={`inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 font-mono text-[11px] ${tone}`}>{children}</span>;
}

export function Empty({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <div className="rounded-2xl border border-dashed border-slate-300 p-10 text-center">
      <p className="font-medium text-slate-700">{title}</p>
      {children && <div className="mt-2 text-sm text-slate-500">{children}</div>}
    </div>
  );
}
