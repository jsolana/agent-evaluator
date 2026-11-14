import type { Change, Outcome } from "../../api/types";
import { changeView, outcomeView } from "../../domain/views";
import { Chip } from "../../ui/Card";
import { Pill } from "../../ui/Verdict";

export function OutcomePill({ outcome }: { outcome: Outcome }) {
  const o = outcomeView[outcome];
  return <Pill tone={o.tone}>{o.icon} {o.label}</Pill>;
}

export function ChangePill({ change }: { change: Change }) {
  const c = changeView[change];
  return c ? <Pill tone={c.tone}>{c.label}</Pill> : null;
}

export function ToolDelta({ delta }: { delta: Record<string, number> }) {
  const items = Object.entries(delta).filter(([, d]) => d !== 0);
  if (!items.length) return <span className="text-xs text-slate-400">=</span>;
  return (
    <div className="flex flex-wrap gap-1">
      {items.map(([n, d]) => <Chip key={n} tone={d > 0 ? "bg-amber-50 text-amber-800" : "bg-sky-50 text-sky-700"}>{n} {d > 0 ? `+${d}` : d}</Chip>)}
    </div>
  );
}
