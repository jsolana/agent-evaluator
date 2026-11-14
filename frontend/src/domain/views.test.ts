import { analysisView, criterionSummary, criteriaView, turnVerdict, needsReview } from "./views";
import type { TurnReport } from "../api/types";

const turn = (t: Partial<TurnReport>) => ({ status: "completed", passed: true, ...t }) as TurnReport;

test("turnVerdict maps turn state to a single verdict vocabulary", () => {
  expect(turnVerdict(turn({}))).toBe("pass");
  expect(turnVerdict(turn({ passed: false }))).toBe("fail");
  expect(turnVerdict(turn({ status: "execution_error" }))).toBe("error");
  expect(turnVerdict(turn({ status: "not_run" }))).toBe("not_run");
  expect(turnVerdict(turn({ status: "running" }))).toBe("not_run");
});

test("criteria are described declaratively, with a fallback for unknown types", () => {
  expect(criteriaView.tools_required.label).toBe("Tools requeridas");
  expect(criterionSummary("tools_required", { tools: ["search", "getSource"] })).toBe("search, getSource");
  expect(criterionSummary("regex", { pattern: "utm_", negate: true })).toBe("no debe aparecer /utm_/");
  expect(criterionSummary("sources_retrieved", { urls: ["a", "b"], match: "any" })).toBe("alguna de 2 URLs");
  expect(criterionSummary("llm_judge", { expected: "Describe ambos" })).toBe("Describe ambos");
  expect(criterionSummary("new_type", { x: 1 })).toBe('{"x":1}');
});

test("an analysis that contradicts the verdict needs review", () => {
  expect(analysisView.false_positive.contradicts).toBe(true);
  expect(analysisView.correct.contradicts).toBe(false);
  expect(needsReview(turn({ latestAnalysis: { verdict: "judge_too_lax" } as TurnReport["latestAnalysis"] }))).toBe(true);
  expect(needsReview(turn({ latestAnalysis: { verdict: "real_failure" } as TurnReport["latestAnalysis"] }))).toBe(false);
  expect(needsReview(turn({}))).toBe(false);
});
