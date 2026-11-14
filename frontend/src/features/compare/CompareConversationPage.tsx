import { Link, useParams, useSearchParams } from "react-router-dom";
import { useScenarioComparison, useTurnComparison } from "../../api/hooks";
import type { TurnReport } from "../../api/types";
import { duration, signed, tokens } from "../../domain/format";
import { turnVerdict } from "../../domain/views";
import { wordDiff } from "../../domain/wordDiff";
import { Delta } from "../../ui/Card";
import { Drawer } from "../../ui/Overlay";
import { QueryState } from "../../ui/QueryState";
import { Markdown } from "../../ui/Text";
import { VerdictBadge } from "../../ui/Verdict";
import { AnalysisBadge } from "../analysis/Analysis";
import { CriteriaResults, expectedUrls } from "../conversation/Criteria";
import { TurnCalls } from "../traces/Traces";
import { CompareHeader } from "./ComparePage";
import { ChangePill, OutcomePill, ToolDelta } from "./parts";

function Side({ turn, title }: { turn: TurnReport | null; title: string }) {
  if (!turn) return <div className="card grid place-items-center p-6 text-sm text-slate-400">{title}: no existe</div>;
  return (
    <div className="card space-y-2 p-4">
      <p className="flex items-center gap-2 text-xs text-slate-500"><VerdictBadge verdict={turnVerdict(turn)} size="sm" /> {title} · {duration(turn.durationMs)} · {tokens(turn.tokens)} tok <AnalysisBadge analysis={turn.latestAnalysis} /></p>
      <Markdown>{turn.answer || "—"}</Markdown>
    </div>
  );
}

function TurnComparisonDrawer({ id, baseId, scenario, turn, onClose }: { id: number; baseId: number; scenario: string; turn: number; onClose: () => void }) {
  const q = useTurnComparison(id, baseId, scenario, turn);
  return (
    <Drawer title={`Turno ${turn} · base vs objetivo`} onClose={onClose} wide>
      <QueryState query={q}>
        {(d) => (
          <>
            <div className="flex flex-wrap items-center gap-2"><OutcomePill outcome={d.outcome} /><ChangePill change={d.change} /><span className="text-xs text-slate-500">Δ tools</span><ToolDelta delta={d.toolDelta} /></div>
            {d.base && d.target && (
              <section className="card p-4" aria-label="Diferencias en la respuesta">
                <p className="label mb-2">Diferencias en la respuesta</p>
                <p className="whitespace-pre-wrap text-sm leading-relaxed">
                  {d.base.answer === d.target.answer && <span className="mr-2 text-xs font-medium text-slate-400">Sin cambios en la respuesta ·</span>}
                  {wordDiff(d.base.answer, d.target.answer).map((p, i) =>
                    p.op === "same" ? p.text : <span key={i} className={p.op === "add" ? "bg-emerald-100 text-emerald-900" : "bg-rose-100 text-rose-900 line-through"}>{p.text}</span>,
                  )}
                </p>
              </section>
            )}
            <div className="grid gap-4 lg:grid-cols-2">
              {(["base", "target"] as const).map((side) => {
                const t = d[side];
                return (
                  <div key={side} className="space-y-3">
                    <h3 className="label">{side === "base" ? "Base" : "Objetivo"}</h3>
                    {t ? (
                      <>
                        <CriteriaResults criteria={t.criteria} results={t.criterionResults} />
                        <div className="card p-3"><TurnCalls turn={t} expectedUrls={expectedUrls(t.criteria)} detailed /></div>
                        {t.annotations.map((a) => <p key={a.id} className="rounded-xl bg-violet-50 p-3 text-sm text-violet-900">✎ <b>{a.author}</b>: {a.body}</p>)}
                      </>
                    ) : <p className="text-sm text-slate-400">No existe en esta evaluación</p>}
                  </div>
                );
              })}
            </div>
          </>
        )}
      </QueryState>
    </Drawer>
  );
}

export default function CompareConversationPage() {
  const { id, baseId, key } = useParams();
  const [t, b] = [Number(id), Number(baseId)];
  const [params, setParams] = useSearchParams();
  const turn = params.get("turn") ? Number(params.get("turn")) : undefined;
  const cmp = useScenarioComparison(t, b, key!);
  return (
    <div className="space-y-6">
      <div className="space-y-2">
        <Link to={`/evaluations/${t}/compare/${b}`} className="text-xs text-slate-500 hover:text-indigo-600">← Comparación</Link>
        <h1 className="font-mono text-xl font-semibold text-slate-900">{key}</h1>
        <CompareHeader id={t} baseId={b} />
      </div>
      <QueryState query={cmp}>
        {(c) => (
          <>
            <div className="flex items-center gap-2"><OutcomePill outcome={c.outcome} /><ChangePill change={c.change} /></div>
            <ol className="space-y-5">
              {c.turns.map((d) => (
                <li key={d.index} className="space-y-2">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="grid h-6 w-6 place-items-center rounded-full bg-indigo-50 text-xs font-semibold text-indigo-700">{d.index}</span>
                    <p className="text-sm font-medium text-slate-800">{(d.target ?? d.base)?.question}</p>
                    <OutcomePill outcome={d.outcome} /><ChangePill change={d.change} />
                    <Delta value={d.deltaTokens} good="down" text={`${signed(d.deltaTokens, tokens)} tok`} />
                    <button className="btn-ghost ml-auto text-xs" onClick={() => setParams({ turn: String(d.index) })} aria-label={`Comparar turno ${d.index}`}>Comparar turno →</button>
                  </div>
                  <div className="grid gap-3 lg:grid-cols-2">
                    <Side turn={d.base} title="Base" />
                    <Side turn={d.target} title="Objetivo" />
                  </div>
                </li>
              ))}
            </ol>
          </>
        )}
      </QueryState>
      {turn !== undefined && <TurnComparisonDrawer id={t} baseId={b} scenario={key!} turn={turn} onClose={() => setParams({})} />}
    </div>
  );
}
