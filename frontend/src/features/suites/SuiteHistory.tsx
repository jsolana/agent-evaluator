import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { CartesianGrid, Legend, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { useEvaluations } from "../../api/hooks";
import { date, pct, tokens } from "../../domain/format";
import { evaluationStatusTone } from "../../domain/views";
import { Card, Empty } from "../../ui/Card";
import { QueryState } from "../../ui/QueryState";

/** Evolution of a suite across all its versions, and the entry point to compare two runs. */
export function SuiteHistory({ name }: { name: string }) {
  const evaluations = useEvaluations(name);
  const [picked, setPicked] = useState<number[]>([]);
  const navigate = useNavigate();
  const toggle = (id: number) => setPicked((p) => (p.includes(id) ? p.filter((x) => x !== id) : [...p, id].slice(-2)));
  const compare = () => {
    const [base, target] = [...picked].sort((a, b) => a - b);
    navigate(`/evaluations/${target}/compare/${base}`);
  };
  return (
    <QueryState query={evaluations}>
      {(list) =>
        list.length === 0 ? (
          <Empty title="Sin evaluaciones">Lanza la primera evaluación de esta suite.</Empty>
        ) : (
          <div className="space-y-4">
            {list.length > 1 && (
              <Card title="Tendencia">
                <div className="h-56">
                  <ResponsiveContainer>
                    <LineChart data={list.map((e) => ({ name: `${e.evaluation.label} (v${e.evaluation.suiteVersion})`, escenarios: e.summary.scenarioPassRate * 100, turnos: e.summary.turnPassRate * 100 }))}>
                      <CartesianGrid strokeDasharray="3 3" stroke="#e2e8f0" />
                      <XAxis dataKey="name" tick={{ fontSize: 11 }} />
                      <YAxis domain={[0, 100]} unit="%" tick={{ fontSize: 11 }} />
                      <Tooltip formatter={(v: number) => `${Math.round(v)}%`} />
                      <Legend />
                      <Line type="monotone" dataKey="escenarios" name="Pass rate escenarios" stroke="#6366f1" strokeWidth={2} />
                      <Line type="monotone" dataKey="turnos" name="Pass rate turnos" stroke="#10b981" strokeWidth={2} />
                    </LineChart>
                  </ResponsiveContainer>
                </div>
              </Card>
            )}
            <Card title="Evaluaciones" actions={<button className="btn-primary" disabled={picked.length !== 2} onClick={compare}>⇄ Comparar seleccionadas</button>}>
              <table className="w-full text-sm">
                <thead className="text-left text-xs text-slate-500">
                  <tr><th className="w-8" /><th>Evaluación</th><th>Versión</th><th>Estado</th><th>Escenarios</th><th>Turnos</th><th>Tokens</th><th>Fecha</th></tr>
                </thead>
                <tbody className="divide-y divide-slate-100">
                  {[...list].reverse().map(({ evaluation: e, summary: s }) => (
                    <tr key={e.id} className="hover:bg-slate-50">
                      <td className="py-2"><input type="checkbox" aria-label={`Seleccionar ${e.label}`} checked={picked.includes(e.id)} onChange={() => toggle(e.id)} /></td>
                      <td><Link className="font-medium text-indigo-600 hover:underline" to={`/evaluations/${e.id}`}>{e.label}</Link></td>
                      <td>v{e.suiteVersion}</td>
                      <td><span className={`rounded-full px-2 py-0.5 text-xs ${evaluationStatusTone[e.status]}`}>{e.status}</span></td>
                      <td>{pct(s.scenarioPassRate)} <span className="text-xs text-slate-400">({s.scenarios.passed}/{s.scenarios.passed + s.scenarios.failed})</span></td>
                      <td>{pct(s.turnPassRate)}</td>
                      <td>{tokens(s.tokens)}</td>
                      <td className="text-slate-500">{date(e.createdAt)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </Card>
          </div>
        )
      }
    </QueryState>
  );
}
