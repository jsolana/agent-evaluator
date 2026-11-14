import { useState } from "react";
import { useImportSuite } from "../../api/hooks";
import { ApiError } from "../../api/client";
import type { Suite } from "../../api/types";
import { Modal } from "../../ui/Overlay";

/** Import a suite or save a new version: same endpoint, same validation feedback. */
export function YamlModal({ title, initial = "", onClose, onSaved }: {
  title: string; initial?: string; onClose: () => void; onSaved: (suite: Suite) => void;
}) {
  const [yaml, setYaml] = useState(initial);
  const save = useImportSuite();
  const err = save.error instanceof ApiError ? save.error : null;

  const readFile = async (file?: File) => file && setYaml(await file.text());
  return (
    <Modal title={title} onClose={onClose}>
      <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); save.mutate(yaml, { onSuccess: onSaved }); }}>
        <label className="block">
          <span className="label">Suite YAML</span>
          <textarea
            aria-label="Suite YAML" value={yaml} onChange={(e) => setYaml(e.target.value)} spellCheck={false}
            onDrop={(e) => { e.preventDefault(); readFile(e.dataTransfer.files[0]); }}
            placeholder={"name: Mi suite\nscenarios:\n  - key: ...\n    turns: [...]\n\n(o arrastra aquí un fichero .yaml)"}
            className="input mt-1 h-80 font-mono text-xs leading-relaxed"
          />
        </label>
        <input type="file" accept=".yaml,.yml" aria-label="Subir fichero YAML" onChange={(e) => readFile(e.target.files?.[0])} className="text-xs text-slate-500" />
        {err && (
          <div role="alert" className="rounded-xl bg-rose-50 p-3 text-sm text-rose-800 ring-1 ring-rose-200">
            <p className="font-medium">{err.message}</p>
            <ul className="mt-1 list-disc space-y-0.5 pl-5 font-mono text-xs">{err.details.map((d) => <li key={d}>{d}</li>)}</ul>
          </div>
        )}
        <div className="flex justify-end gap-2">
          <button type="button" className="btn-ghost" onClick={onClose}>Cancelar</button>
          <button type="submit" className="btn-primary" disabled={!yaml.trim() || save.isPending}>{save.isPending ? "Validando…" : "Guardar suite"}</button>
        </div>
      </form>
    </Modal>
  );
}
