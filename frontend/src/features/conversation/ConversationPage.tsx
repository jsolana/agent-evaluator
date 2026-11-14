import { Link, useParams, useSearchParams } from "react-router-dom";
import { useScenario } from "../../api/hooks";
import type { ScenarioReport } from "../../api/types";
import { duration, tokens } from "../../domain/format";
import { criterionLabel, scenarioStatusView, turnVerdict } from "../../domain/views";
import { Chip } from "../../ui/Card";
import { QueryState } from "../../ui/QueryState";
import { Markdown } from "../../ui/Text";
import { Pill, VerdictBadge, VerdictStrip } from "../../ui/Verdict";
import { AnalysisBadge } from "../analysis/Analysis";
import { ConversationTrace } from "../traces/Traces";
import { TurnDrawer } from "./TurnDrawer";

function Transcript({ scenario, onOpen }: { scenario: ScenarioReport; onOpen: (n: number) => void }) {
  return (
    <ol className="space-y-6">
      {scenario.turns.map((t) => (
        <li key={t.index} className="space-y-3">
          <div className="flex justify-end">
            <div className="max-w-2xl rounded-2xl rounded-br-sm bg-indigo-600 px-4 py-2.5 text-sm text-white shadow-sm">
              <span className="mb-0.5 block text-[10px] font-semibold uppercase tracking-wide text-indigo-200">Turno {t.index} · usuario</span>
              {t.question}
            </div>
          </div>
          <div className="flex gap-3">
            <VerdictBadge verdict={turnVerdict(t)} />
            <div className="card max-w-3xl flex-1 rounded-tl-sm p-4">
              {t.status === "completed" ? <Markdown>{t.answer}</Markdown> : <p className="text-sm italic text-slate-400">{t.error || "Turno no ejecutado"}</p>}
              <footer className="mt-3 flex flex-wrap items-center gap-1.5 border-t border-slate-100 pt-3">
                {t.criterionResults.map((c) => (
                  <Pill key={c.id} tone={c.passed && !c.error ? "bg-emerald-50 text-emerald-700 ring-emerald-200" : "bg-rose-50 text-rose-700 ring-rose-200"} title={c.explanation}>
                    {c.passed && !c.error ? "✓" : "✕"} {criterionLabel(c.criterionType)}
                  </Pill>
                ))}
                {t.tools.sequence.map((n, i) => <Chip key={i}>{n}</Chip>)}
                <AnalysisBadge analysis={t.latestAnalysis} />
                {t.annotations.length > 0 && <Pill tone="bg-violet-50 text-violet-700 ring-violet-200">✎ {t.annotations.length}</Pill>}
                <span className="ml-auto text-xs text-slate-400">{duration(t.durationMs)} · {tokens(t.tokens)} tok</span>
                <button className="btn-ghost text-xs" onClick={() => onOpen(t.index)} aria-label={`Ver detalle del turno ${t.index}`}>Detalle →</button>
              </footer>
            </div>
          </div>
        </li>
      ))}
    </ol>
  );
}

export default function ConversationPage() {
  const { id, key } = useParams();
  const evalId = Number(id);
  const [params, setParams] = useSearchParams();
  const view = params.get("view") ?? "transcript";
  const turn = params.get("turn") ? Number(params.get("turn")) : undefined;
  const set = (k: string, v?: string) => setParams((p) => (v === undefined ? p.delete(k) : p.set(k, v), p));
  const scenario = useScenario(evalId, key!);

  return (
    <QueryState query={scenario}>
      {(s) => (
        <div className="space-y-6">
          <div className="space-y-3">
            <Link to={`/evaluations/${evalId}`} className="text-xs text-slate-500 hover:text-indigo-600">← Evaluación</Link>
            <div className="flex flex-wrap items-center justify-between gap-4">
              <div>
                <h1 className="flex items-center gap-3 font-mono text-xl font-semibold text-slate-900">
                  {s.key} <Pill tone={scenarioStatusView[s.status].tone}>{scenarioStatusView[s.status].label}</Pill>
                </h1>
                <p className="text-sm text-slate-500">{s.name}{s.firstFailedTurn > 0 && ` · se rompe en el turno ${s.firstFailedTurn}`}</p>
              </div>
              <div className="flex items-center gap-4">
                <VerdictStrip verdicts={s.turns.map(turnVerdict)} onSelect={(n) => set("turn", String(n))} />
                <span className="text-xs text-slate-500">{duration(s.totalMs)} · {tokens(s.totalTokens)} tokens</span>
              </div>
            </div>
            {s.error && <p role="alert" className="rounded-xl bg-amber-50 p-3 text-sm text-amber-800">{s.error}</p>}
            <div role="tablist" className="flex gap-1 border-b border-slate-200">
              {[["transcript", "Conversación"], ["traces", "Trazas"]].map(([v, label]) => (
                <button key={v} role="tab" aria-selected={view === v} onClick={() => set("view", v)}
                  className={`-mb-px border-b-2 px-4 py-2 text-sm font-medium ${view === v ? "border-indigo-600 text-indigo-600" : "border-transparent text-slate-500 hover:text-slate-800"}`}>{label}</button>
              ))}
            </div>
          </div>
          {view === "traces" ? <ConversationTrace scenario={s} onOpenTurn={(n) => set("turn", String(n))} /> : <Transcript scenario={s} onOpen={(n) => set("turn", String(n))} />}
          {turn !== undefined && <TurnDrawer id={evalId} scenario={s.key} turn={turn} onClose={() => set("turn")} />}
        </div>
      )}
    </QueryState>
  );
}
