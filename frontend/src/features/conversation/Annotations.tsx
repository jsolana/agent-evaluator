import { useState } from "react";
import { useAddAnnotation } from "../../api/hooks";
import type { Annotation } from "../../api/types";
import { date } from "../../domain/format";
import { ANNOTATION_TAGS } from "../../domain/views";

/** Chronological reasoning thread of the reviewers on a turn. */
export function Annotations({ id, scenario, turn, items }: { id: number; scenario: string; turn: number; items: Annotation[] }) {
  const [body, setBody] = useState("");
  const [tags, setTags] = useState<string[]>([]);
  const add = useAddAnnotation(id, scenario, turn);
  const toggle = (t: string) => setTags((s) => (s.includes(t) ? s.filter((x) => x !== t) : [...s, t]));
  return (
    <section aria-label="Anotaciones" className="card space-y-3 p-5">
      <h3 className="text-sm font-semibold text-slate-700">Anotaciones ({items.length})</h3>
      <ol className="space-y-2">
        {items.map((a) => (
          <li key={a.id} className="rounded-xl bg-slate-50 p-3 text-sm">
            <p className="text-xs text-slate-500"><b className="text-slate-700">{a.author}</b> · {date(a.createdAt)} {a.tags?.map((t) => <span key={t} className="ml-1 rounded bg-violet-100 px-1.5 text-violet-700">{t}</span>)}</p>
            <p className="mt-1 whitespace-pre-wrap text-slate-800">{a.body}</p>
          </li>
        ))}
      </ol>
      <form className="space-y-2" onSubmit={(e) => {
        e.preventDefault();
        add.mutate({ author: "reviewer", body, tags }, { onSuccess: () => (setBody(""), setTags([])) });
      }}>
        <textarea aria-label="Nueva anotación" className="input h-20" placeholder="¿Qué ha pasado en este turno?" value={body} onChange={(e) => setBody(e.target.value)} />
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex flex-wrap gap-1" role="group" aria-label="Tags">
            {ANNOTATION_TAGS.map((t) => (
              <button key={t} type="button" aria-pressed={tags.includes(t)} onClick={() => toggle(t)}
                className={`rounded-full px-2 py-0.5 text-xs ring-1 ${tags.includes(t) ? "bg-violet-600 text-white ring-violet-600" : "text-slate-600 ring-slate-300 hover:bg-slate-100"}`}>{t}</button>
            ))}
          </div>
          <button type="submit" className="btn-primary" disabled={!body.trim() || add.isPending}>Anotar</button>
        </div>
      </form>
    </section>
  );
}
