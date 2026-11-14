import { lazy, Suspense } from "react";
import { Link, Navigate, NavLink, Route, Routes } from "react-router-dom";
import { useHealth } from "./api/hooks";

const SuitesPage = lazy(() => import("./features/suites/SuitesPage"));
const SuitePage = lazy(() => import("./features/suites/SuitePage"));
const EvaluationPage = lazy(() => import("./features/evaluation/EvaluationPage"));
const ConversationPage = lazy(() => import("./features/conversation/ConversationPage"));
const ComparePage = lazy(() => import("./features/compare/ComparePage"));
const CompareConversationPage = lazy(() => import("./features/compare/CompareConversationPage"));

function HealthIndicator() {
  const { data, isError } = useHealth();
  const checks = Object.entries(data?.checks ?? {});
  const warn = isError || checks.some(([, v]) => v !== "ok");
  return (
    <div className="flex items-center gap-3 text-xs" aria-label="Estado del sistema">
      {isError && <span className="text-rose-600">Backend no disponible</span>}
      {checks.map(([name, v]) => (
        <span key={name} title={v} className="flex items-center gap-1.5 text-slate-500">
          <span className={`h-2 w-2 rounded-full ${v === "ok" ? "bg-emerald-500" : v.startsWith("warning") ? "bg-amber-400" : "bg-rose-500"}`} />
          {name}
        </span>
      ))}
      {warn && !isError && <span className="text-amber-700">revisa la configuración</span>}
    </div>
  );
}

export function App() {
  return (
    <div className="min-h-screen">
      <header className="sticky top-0 z-30 border-b border-slate-200 bg-white/80 backdrop-blur">
        <div className="mx-auto flex max-w-7xl items-center justify-between px-6 py-3">
          <Link to="/suites" className="flex items-center gap-2 font-semibold text-slate-900">
            <span className="grid h-8 w-8 place-items-center rounded-xl bg-gradient-to-br from-indigo-500 to-violet-500 text-white">⇄</span>
            Multi-turn Evaluator
          </Link>
          <nav className="flex items-center gap-6">
            <NavLink to="/suites" className={({ isActive }) => `text-sm font-medium ${isActive ? "text-indigo-600" : "text-slate-600 hover:text-slate-900"}`}>
              Suites
            </NavLink>
            <HealthIndicator />
          </nav>
        </div>
      </header>
      <main className="mx-auto max-w-7xl px-6 py-8">
        <Suspense fallback={<div className="h-40 animate-pulse rounded-2xl bg-slate-200/60" />}>
          <Routes>
            <Route path="/" element={<Navigate to="/suites" replace />} />
            <Route path="/suites" element={<SuitesPage />} />
            <Route path="/suites/:suiteId" element={<SuitePage />} />
            <Route path="/evaluations/:id" element={<EvaluationPage />} />
            <Route path="/evaluations/:id/scenarios/:key" element={<ConversationPage />} />
            <Route path="/evaluations/:id/compare/:baseId" element={<ComparePage />} />
            <Route path="/evaluations/:id/compare/:baseId/scenarios/:key" element={<CompareConversationPage />} />
            <Route path="*" element={<p>Página no encontrada</p>} />
          </Routes>
        </Suspense>
      </main>
    </div>
  );
}
