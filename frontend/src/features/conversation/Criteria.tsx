import type { Criterion, CriterionResult } from "../../api/types";
import { tokens } from "../../domain/format";
import { criterionLabel, criterionSummary } from "../../domain/views";
import { VerdictBadge } from "../../ui/Verdict";

/** Each declared criterion with its result, explanation and evidence. */
export function CriteriaResults({ criteria, results }: { criteria: Criterion[]; results: CriterionResult[] }) {
  return (
    <ul className="space-y-2" aria-label="Criterios">
      {results.map((r) => {
        const def = criteria.find((c) => c.id === r.criterionId);
        return (
          <li key={r.id} className="rounded-xl border border-slate-200 bg-white p-3">
            <div className="flex items-start gap-3">
              <VerdictBadge verdict={r.error ? "error" : r.passed ? "pass" : "fail"} size="sm" />
              <div className="min-w-0 flex-1 space-y-1">
                <p className="text-sm font-medium text-slate-800">
                  {criterionLabel(r.criterionType)} <span className="font-normal text-slate-500">· {criterionSummary(r.criterionType, def?.params)}</span>
                </p>
                <p className="text-sm text-slate-600">{r.error || r.explanation}</p>
                {r.evidence?.length ? <p className="text-xs text-slate-500">Evidencia: {r.evidence.map((e) => `“${e}”`).join(" · ")}</p> : null}
                <p className="text-[11px] text-slate-400">{r.criterionVersion.includes("/") ? r.criterionVersion : `v${r.criterionVersion}`}{r.costTokens ? ` · ${tokens(r.costTokens)} tokens` : ""}{r.score !== undefined ? ` · score ${r.score}` : ""}</p>
              </div>
            </div>
          </li>
        );
      })}
    </ul>
  );
}

export const expectedUrls = (criteria: Criterion[]) =>
  criteria.filter((c) => c.type === "sources_retrieved").flatMap((c) => (c.params?.urls as string[]) ?? []);
