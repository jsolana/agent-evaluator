import { duration, pct, signed, tokens } from "./format";

test("formats numbers for humans", () => {
  expect(pct(0.5)).toBe("50%");
  expect(pct(2 / 3)).toBe("67%");
  expect(duration(450)).toBe("450 ms");
  expect(duration(41200)).toBe("41.2 s");
  expect(duration(125000)).toBe("2 min 5 s");
  expect(tokens(950)).toBe("950");
  expect(tokens(31400)).toBe("31.4k");
  expect(signed(0.25, pct)).toBe("+25%");
  expect(signed(-1200, tokens)).toBe("−1.2k");
  expect(signed(0, tokens)).toBe("±0");
});
