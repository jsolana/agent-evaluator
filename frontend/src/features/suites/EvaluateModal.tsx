import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useCreateEvaluation } from "../../api/hooks";
import type { Params, Suite } from "../../api/types";
import { Modal } from "../../ui/Overlay";

export function EvaluateModal({ suite, onClose }: { suite: Suite; onClose: () => void }) {
  const [label, setLabel] = useState(`${suite.name} v${suite.version}`);
  const [concurrency, setConcurrency] = useState(2);
  const [config, setConfig] = useState("");
  const create = useCreateEvaluation();
  const navigate = useNavigate();

  let agentConfig: Params | undefined, configError = "";
  try {
    agentConfig = config.trim() ? JSON.parse(config) : undefined;
  } catch {
    configError = "JSON inválido";
  }
  return (
    <Modal title={`Evaluar ${suite.name} v${suite.version}`} onClose={onClose}>
      <form className="space-y-4" onSubmit={(e) => {
        e.preventDefault();
        create.mutate({ suiteId: suite.id, label, concurrency, agentConfig }, { onSuccess: (ev) => navigate(`/evaluations/${ev.id}`) });
      }}>
        <label className="block"><span className="label">Etiqueta</span>
          <input className="input mt-1" value={label} onChange={(e) => setLabel(e.target.value)} aria-label="Etiqueta" required />
        </label>
        <label className="block"><span className="label">Conversaciones en paralelo</span>
          <input className="input mt-1 w-32" type="number" min={1} max={20} value={concurrency} onChange={(e) => setConcurrency(+e.target.value)} aria-label="Conversaciones en paralelo" />
        </label>
        <label className="block"><span className="label">Configuración del agente (JSON, opcional)</span>
          <textarea className="input mt-1 h-24 font-mono text-xs" value={config} onChange={(e) => setConfig(e.target.value)} aria-label="Configuración del agente"
            placeholder='{"property_1": "value_1", "property_2": "value_2"}  — vacío = configuración por defecto del agente' />
          {configError && <span className="text-xs text-rose-600">{configError}</span>}
        </label>
        {create.error && <p role="alert" className="text-sm text-rose-600">{create.error.message}</p>}
        <div className="flex justify-end gap-2">
          <button type="button" className="btn-ghost" onClick={onClose}>Cancelar</button>
          <button type="submit" className="btn-primary" disabled={!!configError || create.isPending}>▶ Lanzar evaluación</button>
        </div>
      </form>
    </Modal>
  );
}
