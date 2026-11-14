import type * as T from "./types";

export class ApiError extends Error {
  constructor(public status: number, message: string, public details: string[] = []) {
    super(message);
  }
}

// HTTP only: no caching or UI concerns. Injected through ApiProvider (DIP).
export function createClient(base = "/api") {
  async function request<R>(method: string, path: string, body?: unknown, raw = false): Promise<R> {
    const res = await fetch(base + path, {
      method,
      headers: body === undefined || typeof body === "string" ? undefined : { "Content-Type": "application/json" },
      body: body === undefined ? undefined : typeof body === "string" ? body : JSON.stringify(body),
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      throw new ApiError(res.status, err.error ?? res.statusText, err.details ?? []);
    }
    return (raw ? res.text() : res.json()) as Promise<R>;
  }
  const get = <R>(path: string) => request<R>("GET", path);
  const turn = (id: number, key: string, n: number) => `/evaluations/${id}/scenarios/${encodeURIComponent(key)}/turns/${n}`;
  const scen = (id: number, key: string) => `/evaluations/${id}/scenarios/${encodeURIComponent(key)}`;

  return {
    health: () => get<T.Health>("/health"),
    suites: () => get<T.SuiteSummary[]>("/suites"),
    suite: (id: number) => get<T.Suite>(`/suites/${id}`),
    suiteSource: (id: number) => request<string>("GET", `/suites/${id}/source`, undefined, true),
    importSuite: (yaml: string) => request<T.Suite>("POST", "/suites", yaml),
    evaluations: (suite: string) => get<T.EvaluationView[]>(`/evaluations?suite=${encodeURIComponent(suite)}`),
    evaluation: (id: number) => get<T.EvaluationView>(`/evaluations/${id}`),
    createEvaluation: (e: T.NewEvaluation) => request<T.Evaluation>("POST", "/evaluations", e),
    scenarios: (id: number) => get<T.ScenarioReport[]>(`/evaluations/${id}/scenarios`),
    scenario: (id: number, key: string) => get<T.ScenarioReport>(scen(id, key)),
    turn: (id: number, key: string, n: number) => get<T.TurnReport>(turn(id, key, n)),
    addAnnotation: (id: number, key: string, n: number, a: T.NewAnnotation) =>
      request<T.Annotation>("POST", `${turn(id, key, n)}/annotations`, a),
    analyses: (id: number, key: string, n: number) => get<T.Analysis[]>(`${turn(id, key, n)}/analysis`),
    analyzeTurn: (id: number, key: string, n: number) => request<T.Analysis>("POST", `${turn(id, key, n)}/analysis`, {}),
    analyzeFailed: (id: number) => request<{ queued: number }>("POST", `/evaluations/${id}/analysis?only=failed`, {}),
    comparison: (id: number, base: number) => get<T.EvaluationComparison>(`/evaluations/${id}/comparison?with=${base}`),
    scenarioComparison: (id: number, base: number, key: string) =>
      get<T.ScenarioComparison>(`${scen(id, key)}/comparison?with=${base}`),
    turnComparison: (id: number, base: number, key: string, n: number) =>
      get<T.TurnDiff>(`${turn(id, key, n)}/comparison?with=${base}`),
  };
}

export type Client = ReturnType<typeof createClient>;
