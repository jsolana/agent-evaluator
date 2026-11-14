import { fireEvent, render, screen } from "@testing-library/react";
import { VerdictStrip } from "./Verdict";

test("VerdictStrip announces each turn and is clickable", () => {
  const onSelect = vi.fn();
  render(<VerdictStrip verdicts={["pass", "fail", "not_run"]} onSelect={onSelect} />);
  expect(screen.getByRole("button", { name: "Turno 2: Falla" })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Turno 3: No ejecutado" })).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Turno 2: Falla" }));
  expect(onSelect).toHaveBeenCalledWith(2);
});
