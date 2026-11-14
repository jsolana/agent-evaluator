import { Link, useParams, useSearchParams } from "react-router-dom";
import { useComparison, useEvaluation } from "../../api/hooks";
import type { Change, Outcome } from "../../api/types";
import { duration, pct, signed, tokens } from "../../domain/format";
import { changeView, outcomeView } from "../../domain/views";
import { Card, Delta, Kpi } from "../../ui/Card";
import { QueryState } from "../../ui/QueryState";
import { VerdictStrip } from "../../ui/Verdict";
import { ChangePill, OutcomePill, ToolDelta } from "./parts";

const OUTCOMES: Outcome[] = ["regression", "improvement", "stable_fail", "stable_pass", "not_comparable"];

export function CompareHeader({ id, baseId }: { id: number; baseId: number }) {
  const target = useEvaluation(id).data?.evaluation;
  const base = useEvaluation(baseId).data?.evaluation;
  return (
    <div className="flex flex-wrap items-center gap-3 text-sm">
      <span className="rounded-xl bg-slate-100 px-3 py-1.5">Base: <Link className="font-medium text-slate-900 hover:underline" to={`/evaluations/${baseId}`}>{base?.label}</Link> <span className="text-slate-500">v{base?.suiteVersion}</span></span>
      <span className="text-slate-400">→</span>
      <span className="rounded-xl bg-indigo-50 px-3 py-1.5">Objetivo: <Link className="font-medium text-indigo-700 hover:underline" to={`/evaluations/${id}`}>{target?.label}</Link> <span className="text-indigo-500">v{target?.suiteVersion}</span></span>
    </div>
  );
}

export default function ComparePage() {
  const id = Number(useParams().id), baseId = Number(useParams().baseId);
  const cmp = useComparison(id, baseId);
  const [params, setParams] = useSearchParams();
  const only = params.get("outcome");
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold text-slate-900">Comparación de evaluaciones</h1>
        <div className="mt-2"><CompareHeader id={id} baseId={baseId} /></div>
      </div>
      <QueryState query={cmp}>
        {(c) => (
          <>
            <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
              <Kpi label="Pass rate escenarios" value={pct(c.target.scenarioPassRate)} delta={<Delta value={c.deltaScenarioPassRate} text={signed(c.deltaScenarioPassRate, pct)} />} hint={`antes ${pct(c.base.scenarioPassRate)}`} />
              <Kpi label="Pass rate turnos" value={pct(c.target.turnPassRate)} delta={<Delta value={c.deltaTurnPassRate} text={signed(c.deltaTurnPassRate, pct)} />} hint={`antes ${pct(c.base.turnPassRate)}`} />
              <Kpi label="Tokens agente" value={tokens(c.target.tokens)} delta={<Delta value={c.deltaTokens} good="down" text={signed(c.deltaTokens, tokens)} />} />
              <Kpi label="Latencia" value={duration(c.target.durationMs)} delta={<Delta value={c.deltaMs} good="down" text={signed(c.deltaMs, duration)} />} />
            </div>
            <div className="flex flex-wrap gap-2" role="group" aria-label="Filtrar por resultado">
              {OUTCOMES.map((o) => (
                <button key={o} aria-pressed={only === o} onClick={() => setParams(only === o ? {} : { outcome: o }, { replace: true })}
                  className={`rounded-full px-3 py-1 text-xs font-medium ring-1 ${only === o ? "ring-2 ring-slate-900" : ""} ${outcomeView[o].tone}`}>
                  {outcomeView[o].icon} {outcomeView[o].label} · {c.outcomes[o] ?? 0}
                </button>
              ))}
              {(Object.entries(c.changes) as [Change, number][]).map(([k, n]) => {
                const v = changeView[k];
                return v && <span key={k} className={`rounded-full px-3 py-1 text-xs ring-1 ${v.tone}`}>{v.label} · {n}</span>;
              })}
            </div>
            <div className="card overflow-hidden">
              <table className="w-full text-sm">
                <thead className="bg-slate-50 text-left text-xs text-slate-500">
                  <tr><th className="px-4 py-2">Escenario</th><th>Resultado</th><th>Base</th><th>Objetivo</th><th>Δ tokens</th><th>Δ latencia</th><th>Δ tools</th></tr>
                </thead>
                <tbody className="divide-y divide-slate-100">
                  {c.scenarios.filter((s) => !only || s.outcome === only).map((s) => (
                    <tr key={s.key} className="hover:bg-indigo-50/40">
                      <td className="px-4 py-2.5">
                        <Link to={`/evaluations/${id}/compare/${baseId}/scenarios/${encodeURIComponent(s.key)}`} className="font-mono font-medium text-indigo-700 hover:underline">{s.key}</Link>{" "}
                        <ChangePill change={s.change} />
                      </td>
                      <td><OutcomePill outcome={s.outcome} /></td>
                      <td>{s.base ? <VerdictStrip verdicts={s.base.turnVerdicts} label={`Base ${s.key}`} /> : "—"}</td>
                      <td>{s.target ? <VerdictStrip verdicts={s.target.turnVerdicts} label={`Objetivo ${s.key}`} /> : "—"}</td>
                      <td><Delta value={s.deltaTokens} good="down" text={signed(s.deltaTokens, tokens)} /></td>
                      <td><Delta value={s.deltaMs} good="down" text={signed(s.deltaMs, duration)} /></td>
                      <td><ToolDelta delta={s.toolDelta} /></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {c.configDiff?.length ? (
              <Card title="Cambios de configuración del agente">
                <table className="w-full text-xs">
                  <thead className="text-left text-slate-500"><tr><th>Campo</th><th>Base</th><th>Objetivo</th></tr></thead>
                  <tbody className="divide-y divide-slate-100 font-mono">
                    {c.configDiff.map((d) => (
                      <tr key={d.path}>
                        <td className="py-1.5 pr-4 text-slate-700">{d.path}</td>
                        <td className="max-w-md truncate pr-4 text-rose-700" title={String(d.base)}>{JSON.stringify(d.base)}</td>
                        <td className="max-w-md truncate text-emerald-700" title={String(d.target)}>{JSON.stringify(d.target)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </Card>
            ) : null}
          </>
        )}
      </QueryState>
    </div>
  );
}
