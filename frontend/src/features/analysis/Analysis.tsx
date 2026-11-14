import { useAddAnnotation, useAnalyses, useAnalyzeTurn } from "../../api/hooks";
import type { Analysis } from "../../api/types";
import { date, pct } from "../../domain/format";
import { analysisView } from "../../domain/views";
import { Pill } from "../../ui/Verdict";

/** Compact marker for tables and transcripts. */
export function AnalysisBadge({ analysis }: { analysis?: Analysis }) {
  if (!analysis) return null;
  const v = analysisView[analysis.verdict];
  return <Pill tone={v.tone} title={analysis.rationale}>{v.contradicts ? "⚠ " : "🔎 "}{v.label}</Pill>;
}

function AnalysisCard({ a }: { a: Analysis }) {
  const v = analysisView[a.verdict];
  return (
    <article className={`rounded-xl p-4 ring-1 ${v.tone}`} aria-label="Dictamen del análisis">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <p className="font-semibold">{v.contradicts && "⚠ "}{v.label}{v.contradicts && " — revisar"}</p>
        <p className="text-xs opacity-80">confianza {pct(a.confidence)} · causa: {a.rootCause} · {a.model} · {date(a.createdAt)}</p>
      </header>
      <p className="mt-2 text-sm">{a.rationale}</p>
      {a.recommendation && <p className="mt-2 text-sm"><b>Recomendación:</b> {a.recommendation}</p>}
    </article>
  );
}

/** Second opinion of the LLM on this turn's evaluation. Advisory: the human decides. */
export function AnalysisPanel({ id, scenario, turn }: { id: number; scenario: string; turn: number }) {
  const history = useAnalyses(id, scenario, turn);
  const analyze = useAnalyzeTurn(id, scenario, turn);
  const annotate = useAddAnnotation(id, scenario, turn);
  const latest = history.data?.at(-1);
  const saveAsAnnotation = (a: Analysis) =>
    annotate.mutate({
      author: "analysis",
      body: `${analysisView[a.verdict].label} (${pct(a.confidence)}): ${a.rationale}${a.recommendation ? `\nRecomendación: ${a.recommendation}` : ""}`,
      tags: [a.verdict === "false_positive" ? "rubric_too_strict" : a.verdict === "judge_too_lax" ? "judge_too_lax" : "analysis"],
    });
  return (
    <section aria-label="Análisis" className="card space-y-3 p-5">
      <header className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-semibold text-slate-700">Análisis (segunda opinión)</h3>
          <p className="text-xs text-slate-500">El LLM revisa pregunta, respuesta, trazas, rúbrica, resultados y anotaciones. No cambia el veredicto.</p>
        </div>
        <button className="btn-primary" onClick={() => analyze.mutate()} disabled={analyze.isPending}>
          {analyze.isPending ? "Analizando…" : "🔎 Analizar"}
        </button>
      </header>
      {analyze.error && <p role="alert" className="text-sm text-rose-600">{analyze.error.message}</p>}
      {latest && (
        <>
          <AnalysisCard a={latest} />
          <div className="flex items-center justify-between text-xs text-slate-500">
            <span>{history.data!.length > 1 && `${history.data!.length} análisis en el historial`}</span>
            <button className="btn-ghost" onClick={() => saveAsAnnotation(latest)} disabled={annotate.isPending || annotate.isSuccess}>
              {annotate.isSuccess ? "✓ Guardado como anotación" : "Guardar como anotación"}
            </button>
          </div>
        </>
      )}
    </section>
  );
}
