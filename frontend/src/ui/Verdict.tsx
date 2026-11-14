import type { Verdict } from "../api/types";
import { verdictView } from "../domain/views";

/** Square badge for one turn verdict; icon + label so colour is never the only signal. */
export function VerdictBadge({ verdict, size = "md" }: { verdict: Verdict; size?: "sm" | "md" }) {
  const v = verdictView[verdict];
  const dim = size === "sm" ? "h-5 w-5 text-[10px]" : "h-6 w-6 text-xs";
  return (
    <span role="img" aria-label={v.label} title={v.label} className={`inline-flex ${dim} items-center justify-center rounded-md font-bold ${v.tone}`}>
      {v.icon}
    </span>
  );
}

/** One cell per turn: where a conversation breaks, at a glance. Optional click per turn. */
export function VerdictStrip({ verdicts, onSelect, label = "Turnos" }: { verdicts: Verdict[]; onSelect?: (turn: number) => void; label?: string }) {
  return (
    <ol aria-label={label} className="flex items-center gap-1">
      {verdicts.map((v, i) => (
        <li key={i}>
          {onSelect ? (
            <button type="button" onClick={() => onSelect(i + 1)} aria-label={`Turno ${i + 1}: ${verdictView[v].label}`} className="rounded-md transition hover:scale-110 focus-visible:ring-2 focus-visible:ring-indigo-500">
              <VerdictBadge verdict={v} size="sm" />
            </button>
          ) : (
            <span aria-label={`Turno ${i + 1}: ${verdictView[v].label}`}>
              <VerdictBadge verdict={v} size="sm" />
            </span>
          )}
        </li>
      ))}
    </ol>
  );
}

/** Rounded label with a ring; tone classes come from the declarative views. */
export function Pill({ tone, children, title }: { tone: string; children: React.ReactNode; title?: string }) {
  return <span title={title} className={`inline-flex items-center gap-1 whitespace-nowrap rounded-full font-sans px-2 py-0.5 text-xs font-medium ring-1 ring-inset ${tone}`}>{children}</span>;
}
