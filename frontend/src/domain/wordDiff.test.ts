import { wordDiff } from "./wordDiff";

test("word diff keeps common words and marks additions and deletions", () => {
  expect(wordDiff("la cuota es S/ 1,200", "la cuota es S/ 1,500 al mes")).toEqual([
    { op: "same", text: "la cuota es S/ " },
    { op: "del", text: "1,200" },
    { op: "add", text: "1,500 al mes" },
  ]);
  expect(wordDiff("igual", "igual")).toEqual([{ op: "same", text: "igual" }]);
});
