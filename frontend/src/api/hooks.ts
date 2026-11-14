import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useApi } from "./ApiProvider";
import type { NewAnnotation, NewEvaluation } from "./types";

// Data only: one hook per resource. Polling while an evaluation runs and cache
// invalidation after mutations live here, never in components.
const LIVE = 2000;
const running = (s?: string) => s === "queued" || s === "running";

export const useHealth = () => {
  const api = useApi();
  return useQuery({ queryKey: ["health"], queryFn: api.health, refetchInterval: 15000, retry: false });
};

export const useSuites = () => {
  const api = useApi();
  return useQuery({ queryKey: ["suites"], queryFn: api.suites });
};

export const useSuite = (id: number) => {
  const api = useApi();
  return useQuery({ queryKey: ["suite", id], queryFn: () => api.suite(id) });
};

export const useSuiteSource = (id: number, enabled: boolean) => {
  const api = useApi();
  return useQuery({ queryKey: ["suite-source", id], queryFn: () => api.suiteSource(id), enabled });
};

export const useImportSuite = () => {
  const api = useApi();
  const qc = useQueryClient();
  return useMutation({ mutationFn: api.importSuite, onSuccess: () => qc.invalidateQueries({ queryKey: ["suites"] }) });
};

export const useEvaluations = (suite?: string) => {
  const api = useApi();
  return useQuery({
    queryKey: ["evaluations", suite],
    queryFn: () => api.evaluations(suite!),
    enabled: !!suite,
    refetchInterval: (q) => (q.state.data?.some((e) => running(e.evaluation.status)) ? LIVE : false),
  });
};

export const useEvaluation = (id: number) => {
  const api = useApi();
  return useQuery({
    queryKey: ["evaluation", id],
    queryFn: () => api.evaluation(id),
    refetchInterval: (q) => (running(q.state.data?.evaluation.status) ? LIVE : false),
  });
};

export const useCreateEvaluation = () => {
  const api = useApi();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (e: NewEvaluation) => api.createEvaluation(e),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["evaluations"] }),
  });
};

/**
 * Scenario results of an evaluation. Keyed by the evaluation status so the
 * final results are always fetched once the run completes, and polled while it runs.
 */
export const useScenarios = (id: number, status?: string) => {
  const api = useApi();
  return useQuery({
    queryKey: ["scenarios", id, status],
    queryFn: () => api.scenarios(id),
    enabled: !!status,
    refetchInterval: running(status) ? LIVE : false,
  });
};

export const useScenario = (id: number, key: string) => {
  const api = useApi();
  return useQuery({
    queryKey: ["scenario", id, key],
    queryFn: () => api.scenario(id, key),
    refetchInterval: (q) => (q.state.data && ["pending", "running"].includes(q.state.data.status) ? LIVE : false),
  });
};

export const useTurn = (id: number, key: string, n?: number) => {
  const api = useApi();
  return useQuery({ queryKey: ["turn", id, key, n], queryFn: () => api.turn(id, key, n!), enabled: n !== undefined });
};

const invalidateTurn = (qc: ReturnType<typeof useQueryClient>, id: number, key: string) =>
  Promise.all(
    [["turn", id, key], ["scenario", id, key], ["scenarios", id], ["analyses", id, key]].map((queryKey) =>
      qc.invalidateQueries({ queryKey }),
    ),
  );

export const useAddAnnotation = (id: number, key: string, n: number) => {
  const api = useApi();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (a: NewAnnotation) => api.addAnnotation(id, key, n, a),
    onSuccess: () => invalidateTurn(qc, id, key),
  });
};

export const useAnalyses = (id: number, key: string, n: number) => {
  const api = useApi();
  return useQuery({ queryKey: ["analyses", id, key, n], queryFn: () => api.analyses(id, key, n) });
};

export const useAnalyzeTurn = (id: number, key: string, n: number) => {
  const api = useApi();
  const qc = useQueryClient();
  return useMutation({ mutationFn: () => api.analyzeTurn(id, key, n), onSuccess: () => invalidateTurn(qc, id, key) });
};

/** Batch analysis runs in the background: refresh the scenarios a few times while results arrive. */
export const useAnalyzeFailed = (id: number) => {
  const api = useApi();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.analyzeFailed(id),
    onSuccess: () =>
      [1500, 4000, 9000, 15000].forEach((ms) => setTimeout(() => qc.invalidateQueries({ queryKey: ["scenarios", id] }), ms)),
  });
};

export const useComparison = (id: number, base: number) => {
  const api = useApi();
  return useQuery({ queryKey: ["comparison", id, base], queryFn: () => api.comparison(id, base) });
};

export const useScenarioComparison = (id: number, base: number, key: string) => {
  const api = useApi();
  return useQuery({ queryKey: ["scenario-comparison", id, base, key], queryFn: () => api.scenarioComparison(id, base, key) });
};

export const useTurnComparison = (id: number, base: number, key: string, n?: number) => {
  const api = useApi();
  return useQuery({
    queryKey: ["turn-comparison", id, base, key, n],
    queryFn: () => api.turnComparison(id, base, key, n!),
    enabled: n !== undefined,
  });
};
