import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useSuites } from "../../api/hooks";
import type { SuiteSummary } from "../../api/types";
import { date } from "../../domain/format";
import { Empty } from "../../ui/Card";
import { QueryState } from "../../ui/QueryState";
import { YamlModal } from "./YamlModal";

const byName = (list: SuiteSummary[]) =>
  Object.entries(list.reduce<Record<string, SuiteSummary[]>>((m, s) => ((m[s.name] ??= []).push(s), m), {})).map(
    ([name, versions]) => ({ name, versions: versions.sort((a, b) => b.version - a.version) }),
  );

export default function SuitesPage() {
  const suites = useSuites();
  const [importing, setImporting] = useState(false);
  const navigate = useNavigate();
  return (
    <div className="space-y-6">
      <div className="flex items-end justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-slate-900">Suites de escenarios</h1>
          <p className="text-sm text-slate-500">Conversaciones multi-turno con criterios por turno, versionadas.</p>
        </div>
        <button className="btn-primary" onClick={() => setImporting(true)}>＋ Importar suite</button>
      </div>
      <QueryState query={suites}>
        {(list) =>
          list.length === 0 ? (
            <Empty title="Todavía no hay suites">Importa un YAML para empezar a evaluar conversaciones.</Empty>
          ) : (
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {byName(list).map(({ name, versions }) => (
                <Link key={name} to={`/suites/${versions[0].id}`} className="card group p-5 transition hover:-translate-y-0.5 hover:shadow-md">
                  <div className="flex items-start justify-between">
                    <h2 className="font-semibold text-slate-900 group-hover:text-indigo-600">{name}</h2>
                    <span className="rounded-full bg-indigo-50 px-2 py-0.5 text-xs font-medium text-indigo-700">v{versions[0].version}</span>
                  </div>
                  <dl className="mt-4 grid grid-cols-3 gap-2 text-center">
                    {[["escenarios", versions[0].scenarioCount], ["versiones", versions.length], ["evaluaciones", versions.reduce((n, v) => n + v.evaluationCount, 0)]].map(([k, v]) => (
                      <div key={k} className="rounded-xl bg-slate-50 py-2">
                        <dd className="text-lg font-semibold text-slate-900">{v}</dd>
                        <dt className="text-[11px] text-slate-500">{k}</dt>
                      </div>
                    ))}
                  </dl>
                  <p className="mt-3 text-xs text-slate-400">Actualizada {date(versions[0].createdAt)}</p>
                </Link>
              ))}
            </div>
          )
        }
      </QueryState>
      {importing && <YamlModal title="Importar suite" onClose={() => setImporting(false)} onSaved={(s) => navigate(`/suites/${s.id}`)} />}
    </div>
  );
}
