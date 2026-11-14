import { Link, useSearchParams } from "react-router-dom";
import type { ScenarioReport, ScenarioStatus } from "../../api/types";
import { duration, tokens } from "../../domain/format";
import { needsReview, scenarioStatusView, turnVerdict } from "../../domain/views";
import { Chip, Empty } from "../../ui/Card";
import { Pill, VerdictStrip } from "../../ui/Verdict";
import { AnalysisBadge } from "../analysis/Analysis";

const FILTERS: (ScenarioStatus | "all" | "review")[] = ["all", "failed", "passed", "execution_error", "review"];
const filterLabel = (f: string) => (f === "all" ? "Todos" : f === "review" ? "⚠ Pendientes de revisión" : scenarioStatusView[f as ScenarioStatus].label);

/** Scenario results with filters kept in the URL (shareable view). */
export function ScenarioTable({ evaluationId, scenarios }: { evaluationId: number; scenarios: ScenarioReport[] }) {
  const [params, setParams] = useSearchParams();
  const filter = params.get("status") ?? "all";
  const q = (params.get("q") ?? "").toLowerCase();
  const set = (k: string, v: string) => setParams((p) => (v ? p.set(k, v) : p.delete(k), p), { replace: true });

  const count = (f: string) => scenarios.filter((s) => (f === "all" ? true : f === "review" ? s.turns.some(needsReview) : s.status === f)).length;
  const rows = scenarios.filter(
    (s) =>
      (filter === "all" || (filter === "review" ? s.turns.some(needsReview) : s.status === filter)) &&
      (!q || [s.key, s.name, ...(s.tags ?? [])].some((v) => v.toLowerCase().includes(q))),
  );
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap gap-1" role="group" aria-label="Filtrar por estado">
          {FILTERS.map((f) => (
            <button key={f} aria-pressed={filter === f} onClick={() => set("status", f === "all" ? "" : f)}
              className={`rounded-full px-3 py-1 text-xs font-medium ring-1 ${filter === f ? "bg-slate-900 text-white ring-slate-900" : "bg-white text-slate-600 ring-slate-200 hover:bg-slate-50"}`}>
              {filterLabel(f)} <span className="opacity-60">{count(f)}</span>
            </button>
          ))}
        </div>
        <input className="input w-64" placeholder="Buscar escenario o tag…" aria-label="Buscar escenario" value={params.get("q") ?? ""} onChange={(e) => set("q", e.target.value)} />
      </div>
      {rows.length === 0 ? (
        <Empty title="Ningún escenario coincide con el filtro" />
      ) : (
        <div className="card overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-slate-50 text-left text-xs text-slate-500">
              <tr><th className="px-4 py-2">Escenario</th><th>Estado</th><th>Turnos</th><th>Se rompe en</th><th>Tools</th><th>Latencia</th><th>Tokens</th><th /></tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {rows.map((s) => (
                <tr key={s.key} className="hover:bg-indigo-50/40">
                  <td className="px-4 py-2.5">
                    <Link to={`/evaluations/${evaluationId}/scenarios/${encodeURIComponent(s.key)}`} className="font-mono font-medium text-indigo-700 hover:underline">{s.key}</Link>
                    <p className="max-w-xs truncate text-xs text-slate-500">{s.name}</p>
                  </td>
                  <td><Pill tone={scenarioStatusView[s.status].tone}>{scenarioStatusView[s.status].label}</Pill></td>
                  <td><VerdictStrip verdicts={s.turns.map(turnVerdict)} label={`Turnos de ${s.key}`} /></td>
                  <td className="text-slate-600">{s.firstFailedTurn ? `turno ${s.firstFailedTurn}` : "—"}</td>
                  <td><div className="flex flex-wrap gap-1">{Object.entries(s.tools.counts).map(([n, c]) => <Chip key={n}>{n}×{c}</Chip>)}</div></td>
                  <td className="tabular-nums text-slate-600">{duration(s.totalMs)}</td>
                  <td className="tabular-nums text-slate-600">{tokens(s.totalTokens)}</td>
                  <td className="pr-4">{s.turns.filter((t) => t.latestAnalysis).slice(0, 1).map((t) => <AnalysisBadge key={t.index} analysis={t.latestAnalysis} />)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
