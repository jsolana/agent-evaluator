import { Link, useNavigate, useParams } from "react-router-dom";
import { useAnalyzeFailed, useEvaluation, useEvaluations, useScenarios } from "../../api/hooks";
import type { EvaluationView } from "../../api/types";
import { date, duration, pct, tokens } from "../../domain/format";
import { criterionLabel, evaluationStatusTone } from "../../domain/views";
import { Bar } from "../../ui/Bar";
import { Card, Chip, Kpi } from "../../ui/Card";
import { QueryState } from "../../ui/QueryState";
import { ScenarioTable } from "./ScenarioTable";

function CompareWith({ ev }: { ev: EvaluationView["evaluation"] }) {
  const all = useEvaluations(ev.suiteName);
  const navigate = useNavigate();
  const others = (all.data ?? []).filter((e) => e.evaluation.id !== ev.id && e.evaluation.status === "completed");
  return (
    <select aria-label="Comparar con" className="input w-56" value="" disabled={!others.length} onChange={(e) => navigate(`/evaluations/${ev.id}/compare/${e.target.value}`)}>
      <option value="">{others.length ? "⇄ Comparar con…" : "Sin otras evaluaciones"}</option>
      {others.map(({ evaluation: o }) => <option key={o.id} value={o.id}>{o.label} · v{o.suiteVersion} · {date(o.createdAt)}</option>)}
    </select>
  );
}

function Breakdown({ summary }: { summary: EvaluationView["summary"] }) {
  const failures = Object.entries(summary.firstFailedTurns).sort(([a], [b]) => +a - +b);
  const maxFail = Math.max(1, ...failures.map(([, n]) => n));
  return (
    <div className="grid gap-4 lg:grid-cols-3">
      <Card title="¿Dónde se rompen las conversaciones?">
        {failures.length === 0 ? <p className="text-sm text-slate-500">Ninguna conversación ha fallado 🎉</p> : (
          <ul className="space-y-2">
            {failures.map(([turn, n]) => (
              <li key={turn} className="grid grid-cols-[4.5rem_1fr_2rem] items-center gap-2 text-xs">
                <span className="text-slate-600">turno {turn}</span>
                <Bar value={n / maxFail} tone="bg-rose-400" label={`Fallan primero en el turno ${turn}`} />
                <span className="tabular-nums text-slate-700">{n}</span>
              </li>
            ))}
          </ul>
        )}
      </Card>
      <Card title="Criterios">
        <ul className="space-y-2">
          {Object.entries(summary.criteria).map(([type, c]) => (
            <li key={type} className="grid grid-cols-[8rem_1fr_3rem] items-center gap-2 text-xs">
              <span className="truncate text-slate-600">{criterionLabel(type)}</span>
              <Bar value={c.total ? c.passed / c.total : 0} tone="bg-emerald-500" label={`${criterionLabel(type)} pasan`} />
              <span className="tabular-nums text-slate-700">{c.passed}/{c.total}</span>
            </li>
          ))}
        </ul>
      </Card>
      <Card title="Uso de tools">
        <div className="flex flex-wrap gap-1.5">{Object.entries(summary.tools).map(([n, c]) => <Chip key={n} tone="bg-indigo-50 text-indigo-700">{n} × {c}</Chip>)}</div>
      </Card>
    </div>
  );
}

export default function EvaluationPage() {
  const id = Number(useParams().id);
  const evaluation = useEvaluation(id);
  const status = evaluation.data?.evaluation.status;
  const live = status === "queued" || status === "running";
  const scenarios = useScenarios(id, status);
  const analyze = useAnalyzeFailed(id);

  return (
    <QueryState query={evaluation}>
      {({ evaluation: ev, summary: s }) => (
        <div className="space-y-6">
          <div className="flex flex-wrap items-end justify-between gap-4">
            <div>
              <Link to={`/suites/${ev.suiteId}`} className="text-xs text-slate-500 hover:text-indigo-600">← {ev.suiteName} v{ev.suiteVersion}</Link>
              <h1 className="flex items-center gap-3 text-2xl font-semibold text-slate-900">
                {ev.label} <span className={`rounded-full px-2.5 py-0.5 text-xs font-medium ${evaluationStatusTone[ev.status]}`}>{ev.status}</span>
              </h1>
              <p className="text-sm text-slate-500">Lanzada {date(ev.createdAt)} · concurrencia {ev.concurrency}{ev.error && ` · ${ev.error}`}</p>
            </div>
            <div className="flex items-center gap-2">
              <button className="btn-ghost ring-1 ring-slate-200" disabled={live || analyze.isPending || s.scenarios.failed === 0} onClick={() => analyze.mutate()}>
                {analyze.isSuccess ? `🔎 ${analyze.data.queued} turnos en análisis` : "🔎 Analizar fallos"}
              </button>
              <CompareWith ev={ev} />
            </div>
          </div>
          {live && (
            <div className="card p-4" aria-live="polite">
              <div className="mb-2 flex justify-between text-xs text-slate-600"><span>Ejecutando conversaciones…</span><span>{pct(s.progress)}</span></div>
              <Bar value={s.progress} label="Progreso" />
            </div>
          )}
          <div className="grid grid-cols-2 gap-4 lg:grid-cols-5">
            <Kpi label="Escenarios" value={pct(s.scenarioPassRate)} hint={`${s.scenarios.passed} pasan · ${s.scenarios.failed} fallan${s.scenarios.executionError ? ` · ${s.scenarios.executionError} con error` : ""}`} />
            <Kpi label="Turnos" value={pct(s.turnPassRate)} hint={`${s.turns.passed}/${s.turns.evaluated} evaluados`} />
            <Kpi label="Tokens agente" value={tokens(s.tokens)} />
            <Kpi label="Tokens juez" value={tokens(s.judgeTokens)} />
            <Kpi label="Latencia acumulada" value={duration(s.durationMs)} />
          </div>
          <Breakdown summary={s} />
          {ev.effectiveConfig && (
            <details className="card p-4">
              <summary className="cursor-pointer text-sm font-semibold text-slate-700">Configuración efectiva del agente</summary>
              <pre className="mt-3 max-h-80 overflow-auto rounded-xl bg-slate-900 p-4 text-xs text-slate-100">{JSON.stringify(ev.effectiveConfig, null, 2)}</pre>
            </details>
          )}
          <QueryState query={scenarios}>{(list) => <ScenarioTable evaluationId={id} scenarios={list} />}</QueryState>
        </div>
      )}
    </QueryState>
  );
}
