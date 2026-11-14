import { useState } from "react";
import type { ScenarioReport, Span, TurnReport } from "../../api/types";
import { duration, tokens } from "../../domain/format";
import { turnVerdict } from "../../domain/views";
import { Bar } from "../../ui/Bar";
import { Chip } from "../../ui/Card";
import { VerdictBadge } from "../../ui/Verdict";

const judgeTokens = (t: TurnReport) => t.criterionResults.reduce((n, c) => n + c.costTokens, 0);
const toolMs = (spans: Span[] = []) => spans.reduce((n, s) => n + s.durationMs, 0);

/** Waterfall of one turn: tool calls in order, then the rest of the turn (model/orchestration). */
export function TurnCalls({ turn, expectedUrls = [], detailed = false }: { turn: TurnReport; expectedUrls?: string[]; detailed?: boolean }) {
  const spans = turn.spans ?? [];
  const total = Math.max(turn.durationMs, toolMs(spans), 1);
  let offset = 0;
  const rest = Math.max(turn.durationMs - toolMs(spans), 0);
  return (
    <ol className="space-y-2" aria-label={`Llamadas del turno ${turn.index}`}>
      {spans.map((s) => {
        const start = offset;
        offset += s.durationMs;
        return (
          <li key={s.seq}>
            <details className="group rounded-lg" open={false}>
              <summary className="grid cursor-pointer grid-cols-[10rem_1fr_5rem_4rem] items-center gap-3 rounded-lg px-2 py-1 text-xs hover:bg-slate-50">
                <span className="truncate font-mono font-medium text-slate-800">{s.name}</span>
                <Bar value={s.durationMs / total} offset={start / total} tone={s.error ? "bg-rose-500" : "bg-indigo-500"} label={`Duración de ${s.name}`} />
                <span className="text-right tabular-nums text-slate-600">{duration(s.durationMs)}</span>
                <span className={s.error ? "text-rose-600" : "text-emerald-600"}>{s.error ? "error" : s.status || "ok"}</span>
              </summary>
              <div className="ml-2 mt-1 space-y-2 border-l-2 border-slate-200 pl-3 text-xs">
                {s.error && <p className="text-rose-600">{s.error}</p>}
                {detailed ? (
                  <>
                    {s.attributes?.arguments !== undefined && <pre className="overflow-x-auto rounded bg-slate-900 p-2 text-[11px] text-slate-100">{JSON.stringify(s.attributes.arguments, null, 2)}</pre>}
                    {s.documents?.length ? (
                      <ul className="space-y-1">
                        {s.documents.map((d) => {
                          const expected = expectedUrls.some((u) => d.url.split("?")[0].replace(/\/$/, "") === u.replace(/\/$/, ""));
                          return (
                            <li key={d.rank} className={`rounded px-2 py-1 ${expected ? "bg-emerald-50 ring-1 ring-emerald-200" : "bg-slate-50"}`}>
                              <span className="font-mono text-slate-400">#{d.rank}</span> {expected && "★ "}
                              <a href={d.url} target="_blank" rel="noreferrer" className="text-indigo-600 hover:underline">{d.title || d.url}</a>
                              <p className="line-clamp-2 text-slate-500">{d.content}</p>
                            </li>
                          );
                        })}
                      </ul>
                    ) : null}
                    {s.attributes?.result !== undefined && <pre className="max-h-40 overflow-auto rounded bg-slate-100 p-2 text-[11px]">{JSON.stringify(s.attributes.result, null, 2)}</pre>}
                  </>
                ) : (
                  <p className="text-slate-500">Abre el turno para ver argumentos y documentos.</p>
                )}
              </div>
            </details>
          </li>
        );
      })}
      <li className="grid grid-cols-[10rem_1fr_5rem_4rem] items-center gap-3 px-2 text-xs text-slate-500">
        <span className="italic">modelo / orquestación</span>
        <Bar value={rest / total} offset={offset / total} tone="bg-slate-400" label="Tiempo fuera de tools" />
        <span className="text-right tabular-nums">{duration(rest)}</span>
        <span />
      </li>
    </ol>
  );
}

/** Conversation → turn → call. Tokens exist per turn only; calls report latency. */
export function ConversationTrace({ scenario, onOpenTurn }: { scenario: ScenarioReport; onOpenTurn: (n: number) => void }) {
  const [open, setOpen] = useState<number[]>([]);
  const calls = scenario.turns.reduce((n, t) => n + (t.spans?.length ?? 0), 0);
  const judge = scenario.turns.reduce((n, t) => n + judgeTokens(t), 0);
  const slowest = Math.max(...scenario.turns.map((t) => t.durationMs), 1);
  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-5">
        {[["Turnos", scenario.turns.length], ["Tool calls", calls], ["Latencia total", duration(scenario.totalMs)], ["Tokens agente", tokens(scenario.totalTokens)], ["Tokens juez", tokens(judge)]].map(([k, v]) => (
          <div key={k} className="card p-3"><p className="label">{k}</p><p className="text-lg font-semibold">{v}</p></div>
        ))}
      </div>
      <div className="flex flex-wrap gap-1.5">{Object.entries(scenario.tools.counts).map(([n, c]) => <Chip key={n}>{n} × {c}</Chip>)}</div>
      <ol className="space-y-2">
        {scenario.turns.map((t) => {
          const expanded = open.includes(t.index);
          return (
            <li key={t.index} className="card p-3">
              <div className="grid grid-cols-[auto_1fr_auto] items-center gap-3">
                <button type="button" aria-expanded={expanded} aria-label={`Turno ${t.index} trazas`} className="flex items-center gap-2 text-sm font-medium"
                  onClick={() => setOpen((o) => (expanded ? o.filter((x) => x !== t.index) : [...o, t.index]))}>
                  <span className="w-4 text-slate-400">{expanded ? "▾" : "▸"}</span>
                  <VerdictBadge verdict={turnVerdict(t)} size="sm" /> Turno {t.index}
                </button>
                <Bar value={t.durationMs / slowest} label={`Latencia del turno ${t.index}`} tone="bg-violet-400" />
                <span className="text-xs tabular-nums text-slate-600">
                  {duration(t.durationMs)} · {tokens(t.tokens)} tok · juez {tokens(judgeTokens(t))} · {t.spans?.length ?? 0} llamadas
                  <button className="ml-2 text-indigo-600 hover:underline" onClick={() => onOpenTurn(t.index)}>detalle</button>
                </span>
              </div>
              {expanded && <div className="mt-3"><TurnCalls turn={t} /></div>}
            </li>
          );
        })}
      </ol>
      <p className="text-xs text-slate-400">Se muestran los pasos que expone el agente evaluado (p. ej. llamadas a tools) con su duración, y los tokens por turno; el tiempo del turno que no cubren esos pasos aparece como “modelo / orquestación”.</p>
    </div>
  );
}
