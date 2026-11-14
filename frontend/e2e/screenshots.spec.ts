import { expect, test } from "@playwright/test";

// Not an assertion test: captures every main view for visual review (SHOTS=dir npx playwright test screenshots).
test.skip(!process.env.SHOTS, "set SHOTS=<dir> to capture screenshots");
const dir = process.env.SHOTS ?? "";

test("capture views", async ({ page, request }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  const yaml = (turn2: string) => `name: Hipotecas demo
scenarios:
  - key: mivivienda
    name: "mortgage | individual_client | complex | multi_turn_follow_up"
    tags: [mortgage]
    turns:
      - question: "¿Qué requisitos tiene el crédito MiVivienda?"
        criteria: [{type: regex, pattern: "answer 1"}, {type: tools_required, tools: [search]}, {type: llm_judge, expected: "Lista los requisitos"}]
      - question: "¿Y cuánto sería la cuota mensual?"
        criteria: [{type: regex, pattern: "${turn2}"}, {type: context_retention, expected: "Sigue hablando de MiVivienda"}]
      - question: "Resume lo anterior en una lista"
        criteria: [{type: regex, pattern: "answer 3"}]
  - key: pos-ferreteria
    name: "business | pos"
    turns:
      - {question: "Quiero afiliar un POS", criteria: [{type: regex, pattern: "answer 1"}]}
      - {question: "¿Cuándo recibo el dinero?", criteria: [{type: regex, pattern: "answer 2"}]}
      - {question: "Resúmelo", criteria: [{type: regex, pattern: "answer 3"}]}
  - key: tarjeta-robada
    name: "security | urgent"
    turns:
      - {question: "Me robaron la tarjeta", criteria: [{type: regex, pattern: "answer 1"}]}
      - {question: "¿Y el celular?", criteria: [{type: regex, pattern: "answer 2"}]}
      - {question: "Gracias", criteria: [{type: regex, pattern: "nunca"}]}
`;
  const v1 = await (await request.post("/api/suites", { data: yaml("answer 9") })).json();
  const e1 = await (await request.post("/api/evaluations", { data: { suiteId: v1.id, label: "baseline", concurrency: 3 } })).json();
  await expect.poll(async () => (await (await request.get(`/api/evaluations/${e1.id}`)).json()).evaluation.status, { timeout: 30000 }).toBe("completed");
  await request.post(`/api/evaluations/${e1.id}/scenarios/mivivienda/turns/2/annotations`, { data: { author: "javi", body: "El corpus dice S/ 1,500: la rúbrica es demasiado estricta", tags: ["rubric_too_strict"] } });
  await request.post(`/api/evaluations/${e1.id}/scenarios/mivivienda/turns/2/analysis`);
  const v2 = await (await request.post("/api/suites", { data: yaml("answer 2") })).json();
  const e2 = await (await request.post("/api/evaluations", { data: { suiteId: v2.id, label: "rubric fix", concurrency: 3 } })).json();
  await expect.poll(async () => (await (await request.get(`/api/evaluations/${e2.id}`)).json()).evaluation.status, { timeout: 30000 }).toBe("completed");

  const shot = async (name: string) => { await page.waitForTimeout(400); await page.screenshot({ path: `${dir}/${name}.png`, fullPage: true }); };
  await page.goto("/suites"); await shot("1-suites");
  await page.goto(`/suites/${v2.id}`); await shot("2-suite");
  await page.goto(`/evaluations/${e1.id}`); await shot("3-evaluation");
  await page.goto(`/evaluations/${e1.id}/scenarios/mivivienda`); await shot("4-conversation");
  await page.goto(`/evaluations/${e1.id}/scenarios/mivivienda?turn=2`); await shot("5-turn-drawer");
  await page.goto(`/evaluations/${e1.id}/scenarios/mivivienda?view=traces`);
  await page.getByRole("button", { name: "Turno 1 trazas" }).click(); await page.getByRole("button", { name: "Turno 2 trazas" }).click(); await shot("6-traces");
  await page.goto(`/evaluations/${e2.id}/compare/${e1.id}`); await shot("7-compare");
  await page.goto(`/evaluations/${e2.id}/compare/${e1.id}/scenarios/mivivienda?turn=2`); await shot("8-compare-turn");
});
