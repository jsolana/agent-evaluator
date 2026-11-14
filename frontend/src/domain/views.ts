import type { AnalysisVerdict, Change, Outcome, Params, ScenarioStatus, TurnReport, Verdict } from "../api/types";

// Declarative presentation tables (OCP): a new criterion, status or outcome is
// a new entry here, components do not change.

export const verdictView: Record<Verdict, { label: string; icon: string; tone: string }> = {
  pass: { label: "Pasa", icon: "✓", tone: "bg-emerald-500 text-white" },
  fail: { label: "Falla", icon: "✕", tone: "bg-rose-500 text-white" },
  error: { label: "Error de ejecución", icon: "!", tone: "bg-amber-400 text-amber-950" },
  not_run: { label: "No ejecutado", icon: "·", tone: "bg-slate-200 text-slate-500" },
};

export const scenarioStatusView: Record<ScenarioStatus, { label: string; tone: string }> = {
  passed: { label: "Pasa", tone: "bg-emerald-50 text-emerald-700 ring-emerald-200" },
  failed: { label: "Falla", tone: "bg-rose-50 text-rose-700 ring-rose-200" },
  execution_error: { label: "Error de ejecución", tone: "bg-amber-50 text-amber-800 ring-amber-200" },
  running: { label: "En curso", tone: "bg-sky-50 text-sky-700 ring-sky-200 animate-pulse" },
  pending: { label: "Pendiente", tone: "bg-slate-50 text-slate-500 ring-slate-200" },
};

export const evaluationStatusTone: Record<string, string> = {
  queued: "bg-slate-100 text-slate-600",
  running: "bg-sky-100 text-sky-700 animate-pulse",
  completed: "bg-emerald-100 text-emerald-700",
  failed: "bg-rose-100 text-rose-700",
};

export const outcomeView: Record<Outcome, { label: string; icon: string; tone: string }> = {
  regression: { label: "Regresión", icon: "↓", tone: "bg-rose-100 text-rose-700 ring-rose-200" },
  improvement: { label: "Mejora", icon: "↑", tone: "bg-emerald-100 text-emerald-700 ring-emerald-200" },
  stable_pass: { label: "Estable ✓", icon: "=", tone: "bg-slate-100 text-slate-600 ring-slate-200" },
  stable_fail: { label: "Estable ✕", icon: "=", tone: "bg-orange-50 text-orange-700 ring-orange-200" },
  not_comparable: { label: "No comparable", icon: "∅", tone: "bg-slate-50 text-slate-400 ring-slate-200" },
};

export const changeView: Record<Change, { label: string; tone: string } | null> = {
  unchanged: null,
  changed: { label: "rúbrica cambiada", tone: "bg-violet-100 text-violet-700 ring-violet-200" },
  added: { label: "nuevo", tone: "bg-sky-100 text-sky-700 ring-sky-200" },
  removed: { label: "eliminado", tone: "bg-slate-200 text-slate-600 ring-slate-300" },
};

export const analysisView: Record<AnalysisVerdict, { label: string; tone: string; contradicts: boolean }> = {
  real_failure: { label: "Fallo real", tone: "bg-rose-50 text-rose-800 ring-rose-200", contradicts: false },
  false_positive: { label: "Posible falso positivo", tone: "bg-amber-50 text-amber-900 ring-amber-300", contradicts: true },
  judge_too_lax: { label: "Juez demasiado laxo", tone: "bg-amber-50 text-amber-900 ring-amber-300", contradicts: true },
  correct: { label: "Veredicto correcto", tone: "bg-emerald-50 text-emerald-800 ring-emerald-200", contradicts: false },
};

const list = (v: unknown) => (Array.isArray(v) ? v.join(", ") : "");

export const criteriaView: Record<string, { label: string; summary: (p: Params) => string }> = {
  llm_judge: { label: "LLM judge", summary: (p) => String(p.expected ?? "") },
  context_retention: { label: "Contexto", summary: (p) => String(p.expected ?? "") },
  tools_required: { label: "Tools requeridas", summary: (p) => list(p.tools) },
  tools_forbidden: { label: "Tools prohibidas", summary: (p) => list(p.tools) },
  sources_retrieved: {
    label: "Fuentes",
    summary: (p) => `${p.match === "any" ? "alguna de" : "todas"} ${Array.isArray(p.urls) ? p.urls.length : 0} URLs`,
  },
  regex: { label: "Regex", summary: (p) => `${p.negate ? "no debe aparecer" : "debe aparecer"} /${String(p.pattern)}/` },
};

export const criterionLabel = (type: string) => criteriaView[type]?.label ?? type;
export const criterionSummary = (type: string, params: Params = {}) =>
  criteriaView[type]?.summary(params) ?? JSON.stringify(params);

export function turnVerdict(t: Pick<TurnReport, "status" | "passed">): Verdict {
  if (t.status === "execution_error") return "error";
  if (t.status !== "completed") return "not_run";
  return t.passed ? "pass" : "fail";
}

export const needsReview = (t: TurnReport) => !!t.latestAnalysis && analysisView[t.latestAnalysis.verdict].contradicts;

export const ANNOTATION_TAGS = ["rubric_too_strict", "judge_too_lax", "agent_bug", "retrieval", "context_lost"];
