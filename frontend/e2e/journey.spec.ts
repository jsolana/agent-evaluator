import { expect, test } from "@playwright/test";

// The fake agent answers "answer <n>: <question>" with one search call.
// Version 1 expects "answer 9" in turn 2 of "pagos" (a wrong rubric); version 2 fixes it.
const suite = (name: string, turn2: string) => `name: ${name}
scenarios:
  - key: pagos
    name: "payments | follow-up"
    turns:
      - question: "¿Cómo pago mi tarjeta?"
        criteria: [{type: regex, pattern: "answer 1"}, {type: tools_required, tools: [search]}]
      - question: "¿Y si pago tarde?"
        criteria: [{type: regex, pattern: "${turn2}"}]
      - question: "Resume lo anterior"
        criteria: [{type: regex, pattern: "answer 3"}]
  - key: estable
    turns:
      - question: "q1"
        criteria: [{type: regex, pattern: "answer 1"}]
      - question: "q2"
        criteria: [{type: regex, pattern: "answer 2"}]
      - question: "q3"
        criteria: [{type: regex, pattern: "answer 3"}]
`;

async function evaluate(page: import("@playwright/test").Page, label: string) {
  await page.getByRole("button", { name: "▶ Evaluar" }).click();
  await page.getByLabel("Etiqueta").fill(label);
  await page.getByRole("button", { name: "▶ Lanzar evaluación" }).click();
  await expect(page).toHaveURL(/\/evaluations\/\d+$/);
  await expect(page.getByRole("heading", { name: new RegExp(`${label}\\s*completed`) })).toBeVisible({ timeout: 30_000 });
}

test("evaluate, review, trace, analyze, fix a rubric and compare", async ({ page }) => {
  // Unique per test: the backend (and its database) is shared by every test of the run.
  const NAME = `E2E Pagos ${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
  await page.goto("/suites");

  // 1. An invalid suite is rejected with every problem located.
  await page.getByRole("button", { name: "＋ Importar suite" }).click();
  await page.getByLabel("Suite YAML").fill(suite(NAME, "answer 9").replace("type: regex, pattern: \"answer 1\"}, {", "type: llm_jugde}, {"));
  await page.getByRole("button", { name: "Guardar suite" }).click();
  await expect(page.getByRole("alert")).toContainText('scenarios[0].turns[0].criteria[0]: unknown type "llm_jugde"');

  // 2. Import the valid suite and evaluate it.
  await page.getByLabel("Suite YAML").fill(suite(NAME, "answer 9"));
  await page.getByRole("button", { name: "Guardar suite" }).click();
  await expect(page.getByRole("heading", { name: new RegExp(NAME) })).toBeVisible();
  await evaluate(page, "baseline");

  // 3. Where it breaks: one scenario fails at turn 2.
  await expect(page.getByText("50%").first()).toBeVisible();
  const row = page.getByRole("row", { name: /pagos/ });
  await expect(row.getByText("turno 2")).toBeVisible();
  await row.getByRole("link", { name: "pagos" }).click();

  // 4. Turn evidence.
  await expect(page.getByRole("button", { name: "Turno 2: Falla" })).toBeVisible();
  await page.getByRole("button", { name: "Ver detalle del turno 2" }).click();
  const drawer = page.getByRole("dialog", { name: "Turno 2 · pagos" });
  await expect(drawer.getByText("pattern not found: answer 9")).toBeVisible();

  // 5. Annotate, 6. ask for a second opinion and keep it.
  await drawer.getByLabel("Nueva anotación").fill("La rúbrica espera answer 9 pero lo correcto es answer 2");
  await drawer.getByRole("button", { name: "rubric_too_strict" }).click();
  await drawer.getByRole("button", { name: "Anotar" }).click();
  await expect(drawer.getByText("La rúbrica espera answer 9 pero lo correcto es answer 2")).toBeVisible();
  await drawer.getByRole("button", { name: "🔎 Analizar" }).click();
  await expect(drawer.getByText(/Posible falso positivo — revisar/)).toBeVisible();
  await drawer.getByRole("button", { name: "Guardar como anotación" }).click();
  await expect(drawer.getByText("✓ Guardado como anotación")).toBeVisible();
  await expect(drawer.getByText("Trazas (1 llamadas)")).toBeVisible();
  await page.keyboard.press("Escape");

  // 7. Conversation trace: totals, per turn latency and calls.
  await page.getByRole("tab", { name: "Trazas" }).click();
  await expect(page.getByText("Tool calls")).toBeVisible();
  await page.getByRole("button", { name: "Turno 2 trazas" }).click();
  const calls = page.getByRole("list", { name: "Llamadas del turno 2" });
  await expect(calls.getByText("search")).toBeVisible();
  await expect(calls.getByText("modelo / orquestación")).toBeVisible();

  // 8. Fix the rubric in a new version and re-run.
  await page.getByRole("link", { name: "← Evaluación" }).click();
  await page.getByRole("link", { name: `← ${NAME} v1` }).click();
  await page.getByRole("button", { name: "✎ Nueva versión" }).click();
  const editor = page.getByLabel("Suite YAML");
  await expect(editor).toHaveValue(/answer 9/);
  await editor.fill((await editor.inputValue()).replace("answer 9", "answer 2"));
  await page.getByRole("button", { name: "Guardar suite" }).click();
  await expect(page.getByRole("combobox", { name: "Versión", exact: true }).locator("option:checked")).toHaveText("v2");
  await evaluate(page, "rubric fix");
  await expect(page.getByText("100%").first()).toBeVisible();

  // 9. Compare at suite, conversation and turn level.
  const select = page.getByLabel("Comparar con");
  const baseline = await select.locator("option", { hasText: "baseline" }).getAttribute("value");
  await select.selectOption(baseline!);
  await expect(page.getByRole("heading", { name: "Comparación de evaluaciones" })).toBeVisible();
  const cmpRow = page.getByRole("row", { name: /pagos/ });
  await expect(cmpRow.getByText("↑ Mejora")).toBeVisible();
  await expect(cmpRow.getByText("rúbrica cambiada")).toBeVisible();
  await cmpRow.getByRole("link", { name: "pagos" }).click();
  await page.getByRole("button", { name: "Comparar turno 2" }).click();
  const turnCmp = page.getByRole("dialog", { name: "Turno 2 · base vs objetivo" });
  await expect(turnCmp.getByText("↑ Mejora")).toBeVisible();
  await expect(turnCmp.getByText(/La rúbrica espera answer 9/)).toBeVisible();
});
