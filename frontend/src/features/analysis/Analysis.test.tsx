import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import { ApiProvider } from "../../api/ApiProvider";
import type { Client } from "../../api/client";
import type { Analysis } from "../../api/types";
import { AnalysisPanel } from "./Analysis";

const verdict: Analysis = {
  id: 1, model: "fake/analysis-v1", verdict: "false_positive", confidence: 0.8, rationale: "La rúbrica exige S/ 1,200",
  rootCause: "rubric", recommendation: "Aceptar S/ 1,500", tokens: 10, createdAt: "2026-11-14T10:00:00Z",
};

function setup() {
  let stored: Analysis[] = [];
  const client = {
    analyses: vi.fn(async () => stored),
    analyzeTurn: vi.fn(async () => ((stored = [verdict]), verdict)),
    addAnnotation: vi.fn(async () => ({ id: 1, author: "analysis", body: "", createdAt: "" })),
  } as unknown as Client;
  render(
    <ApiProvider client={client}>
      <QueryClientProvider client={new QueryClient()}>
        <AnalysisPanel id={7} scenario="mortgage" turn={2} />
      </QueryClientProvider>
    </ApiProvider>,
  );
  return client;
}

test("Analyze shows a contradicting verdict as something to review and can save it as an annotation", async () => {
  const client = setup();
  fireEvent.click(screen.getByRole("button", { name: /Analizar/ }));
  expect(await screen.findByText(/Posible falso positivo — revisar/)).toBeInTheDocument();
  expect(screen.getByText("La rúbrica exige S/ 1,200")).toBeInTheDocument();
  expect(client.analyzeTurn).toHaveBeenCalledWith(7, "mortgage", 2);

  fireEvent.click(screen.getByRole("button", { name: "Guardar como anotación" }));
  expect(await screen.findByText(/Guardado como anotación/)).toBeInTheDocument();
  expect(client.addAnnotation).toHaveBeenCalledWith(7, "mortgage", 2, expect.objectContaining({ tags: ["rubric_too_strict"] }));
});
