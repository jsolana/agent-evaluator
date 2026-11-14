// Contract of the Go backend (internal/domain, internal/report, internal/compare, internal/store).

export type TurnStatus = "pending" | "running" | "completed" | "execution_error" | "not_run";
export type ScenarioStatus = "pending" | "running" | "passed" | "failed" | "execution_error";
export type EvaluationStatus = "queued" | "running" | "completed" | "failed";
export type Verdict = "pass" | "fail" | "error" | "not_run";
export type Change = "unchanged" | "changed" | "added" | "removed";
export type Outcome = "regression" | "improvement" | "stable_pass" | "stable_fail" | "not_comparable";
export type AnalysisVerdict = "real_failure" | "false_positive" | "judge_too_lax" | "correct";

export type Params = Record<string, unknown>;
export interface Criterion { id: number; type: string; params?: Params }
export interface Turn { id: number; index: number; question: string; contentHash: string; criteria?: Criterion[] }
export interface Scenario { id: number; key: string; name: string; tags?: string[]; turns?: Turn[] }
export interface Suite { id: number; name: string; version: number; minTurns: number; createdAt: string; scenarios?: Scenario[] }
export interface SuiteSummary { id: number; name: string; version: number; scenarioCount: number; evaluationCount: number; createdAt: string }

export interface Evaluation {
  id: number; suiteId: number; label: string; status: EvaluationStatus; error?: string; concurrency: number;
  agentConfig?: Params; effectiveConfig?: Params; createdAt: string; startedAt?: string; finishedAt?: string;
  suiteName: string; suiteVersion: number;
}
export interface Summary {
  scenarios: { total: number; passed: number; failed: number; executionError: number; pending: number };
  turns: { total: number; evaluated: number; passed: number };
  scenarioPassRate: number; turnPassRate: number; progress: number;
  tokens: number; judgeTokens: number; durationMs: number;
  tools: Record<string, number>; firstFailedTurns: Record<string, number>;
  criteria: Record<string, { passed: number; total: number }>;
}
export interface EvaluationView { evaluation: Evaluation; summary: Summary }

export interface Document { rank: number; title: string; url: string; content: string }
export interface Span {
  spanId: string; seq: number; kind: string; name: string; durationMs: number; status: string; error?: string;
  attributes?: { arguments?: unknown; result?: unknown }; documents?: Document[];
}
export interface CriterionResult {
  id: number; criterionId: number; criterionType: string; criterionVersion: string; passed: boolean; score?: number;
  explanation: string; evidence?: string[]; costTokens: number; latencyMs: number; error?: string;
}
export interface Annotation { id: number; author: string; body: string; tags?: string[]; createdAt: string }
export interface Analysis {
  id: number; model: string; verdict: AnalysisVerdict; confidence: number; rationale: string;
  rootCause: string; recommendation: string; tokens: number; createdAt: string;
}
export interface ToolUsage { sequence: string[]; counts: Record<string, number> }
export interface TurnReport {
  index: number; question: string; contentHash: string; criteria: Criterion[]; turnResultId?: number;
  status: TurnStatus; passed: boolean; answer: string; tokens: number; durationMs: number; attempts: number; error?: string;
  criterionResults: CriterionResult[]; annotations: Annotation[]; latestAnalysis?: Analysis; tools: ToolUsage; spans?: Span[];
}
export interface ScenarioReport {
  key: string; name: string; tags?: string[]; status: ScenarioStatus; firstFailedTurn: number; conversationRef: string;
  totalTokens: number; totalMs: number; error?: string; turns: TurnReport[]; tools: ToolUsage;
}

export interface ScenarioSide {
  status: ScenarioStatus; firstFailedTurn: number; turnVerdicts: Verdict[]; tokens: number; durationMs: number; tools: Record<string, number>;
}
export interface ScenarioDiff {
  key: string; name: string; change: Change; outcome: Outcome; base: ScenarioSide | null; target: ScenarioSide | null;
  deltaTokens: number; deltaMs: number; toolDelta: Record<string, number>;
}
export interface ConfigChange { path: string; base: unknown; target: unknown }
export interface EvaluationComparison {
  baseId: number; targetId: number; base: Summary; target: Summary;
  deltaScenarioPassRate: number; deltaTurnPassRate: number; deltaTokens: number; deltaMs: number;
  outcomes: Partial<Record<Outcome, number>>; changes: Partial<Record<Change, number>>;
  configDiff: ConfigChange[] | null; scenarios: ScenarioDiff[];
}
export interface TurnDiff {
  index: number; change: Change; outcome: Outcome; base: TurnReport | null; target: TurnReport | null;
  deltaTokens: number; deltaMs: number; toolDelta: Record<string, number>;
}
export interface ScenarioComparison {
  key: string; change: Change; outcome: Outcome; turns: TurnDiff[]; base: ScenarioSide | null; target: ScenarioSide | null;
}
export interface Health { status: string; checks: Record<string, string> }
export interface NewEvaluation { suiteId: number; label: string; agentConfig?: Params; concurrency: number }
export interface NewAnnotation { author: string; body: string; tags: string[] }
