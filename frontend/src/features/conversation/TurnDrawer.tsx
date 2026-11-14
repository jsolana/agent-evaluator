import { useTurn } from "../../api/hooks";
import { duration, tokens } from "../../domain/format";
import { turnVerdict } from "../../domain/views";
import { Drawer } from "../../ui/Overlay";
import { QueryState } from "../../ui/QueryState";
import { Highlight } from "../../ui/Text";
import { VerdictBadge } from "../../ui/Verdict";
import { AnalysisPanel } from "../analysis/Analysis";
import { TurnCalls } from "../traces/Traces";
import { Annotations } from "./Annotations";
import { CriteriaResults, expectedUrls } from "./Criteria";

/** Everything about one turn: evidence, criteria, second opinion, reviewer notes and trace. */
export function TurnDrawer({ id, scenario, turn, onClose }: { id: number; scenario: string; turn: number; onClose: () => void }) {
  const q = useTurn(id, scenario, turn);
  return (
    <Drawer title={`Turno ${turn} · ${scenario}`} onClose={onClose}>
      <QueryState query={q}>
        {(t) => {
          const evidence = t.criterionResults.flatMap((c) => c.evidence ?? []);
          return (
            <>
              <div className="flex items-center gap-3 text-sm text-slate-600">
                <VerdictBadge verdict={turnVerdict(t)} /> {duration(t.durationMs)} · {tokens(t.tokens)} tokens · {t.attempts} intento(s)
                {t.error && <span className="text-rose-600">{t.error}</span>}
              </div>
              <section className="card space-y-3 p-5">
                <p className="label">Usuario</p>
                <p className="text-sm text-slate-800">{t.question}</p>
                <p className="label">Agente {evidence.length > 0 && <span className="normal-case text-amber-600">· evidencia resaltada</span>}</p>
                <Highlight text={t.answer} marks={evidence} />
              </section>
              <section className="space-y-2">
                <h3 className="label">Criterios</h3>
                <CriteriaResults criteria={t.criteria} results={t.criterionResults} />
              </section>
              {t.turnResultId && <AnalysisPanel id={id} scenario={scenario} turn={turn} />}
              {t.turnResultId && <Annotations id={id} scenario={scenario} turn={turn} items={t.annotations} />}
              <section className="card space-y-2 p-5">
                <h3 className="text-sm font-semibold text-slate-700">Trazas ({t.spans?.length ?? 0} llamadas)</h3>
                <TurnCalls turn={t} expectedUrls={expectedUrls(t.criteria)} detailed />
              </section>
            </>
          );
        }}
      </QueryState>
    </Drawer>
  );
}
