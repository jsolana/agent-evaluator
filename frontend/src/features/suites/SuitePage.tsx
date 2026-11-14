import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useSuite, useSuites, useSuiteSource } from "../../api/hooks";
import type { Scenario } from "../../api/types";
import { criterionLabel, criterionSummary } from "../../domain/views";
import { Card } from "../../ui/Card";
import { QueryState } from "../../ui/QueryState";
import { EvaluateModal } from "./EvaluateModal";
import { SuiteHistory } from "./SuiteHistory";
import { YamlModal } from "./YamlModal";

function ScenarioDefinition({ scenario }: { scenario: Scenario }) {
  return (
    <details className="group rounded-xl border border-slate-200 bg-white">
      <summary className="flex cursor-pointer items-center justify-between px-4 py-3">
        <span><span className="font-mono text-sm font-medium text-slate-900">{scenario.key}</span> <span className="text-xs text-slate-500">{scenario.name}</span></span>
        <span className="text-xs text-slate-500">{scenario.turns?.length} turnos</span>
      </summary>
      <ol className="space-y-3 border-t border-slate-100 px-4 py-3">
        {scenario.turns?.map((t) => (
          <li key={t.id} className="flex gap-3">
            <span className="mt-0.5 grid h-6 w-6 shrink-0 place-items-center rounded-full bg-indigo-50 text-xs font-semibold text-indigo-700">{t.index}</span>
            <div className="min-w-0 space-y-1.5">
              <p className="text-sm text-slate-800">{t.question}</p>
              <ul className="flex flex-wrap gap-1.5">
                {t.criteria?.map((c) => (
                  <li key={c.id} title={criterionSummary(c.type, c.params)} className="max-w-md truncate rounded-md bg-slate-100 px-2 py-0.5 text-[11px] text-slate-600">
                    <b>{criterionLabel(c.type)}</b> · {criterionSummary(c.type, c.params)}
                  </li>
                ))}
              </ul>
            </div>
          </li>
        ))}
      </ol>
    </details>
  );
}

export default function SuitePage() {
  const id = Number(useParams().suiteId);
  const suite = useSuite(id);
  const all = useSuites();
  const [modal, setModal] = useState<"evaluate" | "version" | null>(null);
  const source = useSuiteSource(id, modal === "version");
  const navigate = useNavigate();

  return (
    <QueryState query={suite}>
      {(s) => {
        const versions = (all.data ?? []).filter((v) => v.name === s.name).sort((a, b) => b.version - a.version);
        return (
          <div className="space-y-6">
            <div className="flex flex-wrap items-end justify-between gap-4">
              <div>
                <Link to="/suites" className="text-xs text-slate-500 hover:text-indigo-600">← Suites</Link>
                <h1 className="flex items-center gap-3 text-2xl font-semibold text-slate-900">
                  {s.name}
                  <select aria-label="Versión" value={s.id} onChange={(e) => navigate(`/suites/${e.target.value}`)} className="rounded-lg border-slate-300 bg-indigo-50 px-2 py-1 text-sm text-indigo-700">
                    {versions.map((v) => <option key={v.id} value={v.id}>v{v.version}</option>)}
                  </select>
                </h1>
                <p className="text-sm text-slate-500">{s.scenarios?.length} escenarios · mínimo {s.minTurns} turnos</p>
              </div>
              <div className="flex gap-2">
                <button className="btn-ghost ring-1 ring-slate-200" onClick={() => setModal("version")}>✎ Nueva versión</button>
                <button className="btn-primary" onClick={() => setModal("evaluate")}>▶ Evaluar</button>
              </div>
            </div>
            <section aria-label="Historial" className="space-y-3">
              <h2 className="label">Historial de evaluaciones (todas las versiones)</h2>
              <SuiteHistory name={s.name} />
            </section>
            <Card title="Escenarios de esta versión">
              <div className="space-y-2">{s.scenarios?.map((sc) => <ScenarioDefinition key={sc.id} scenario={sc} />)}</div>
            </Card>
            {modal === "evaluate" && <EvaluateModal suite={s} onClose={() => setModal(null)} />}
            {modal === "version" && source.data !== undefined && (
              <YamlModal title={`Nueva versión de ${s.name}`} initial={source.data} onClose={() => setModal(null)}
                onSaved={(v) => { setModal(null); navigate(`/suites/${v.id}`); }} />
            )}
          </div>
        );
      }}
    </QueryState>
  );
}
