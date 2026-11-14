export const pct = (x: number) => `${Math.round(x * 100)}%`;

export function duration(ms: number) {
  if (ms < 1000) return `${Math.round(ms)} ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)} s`;
  return `${Math.floor(ms / 60000)} min ${Math.round((ms % 60000) / 1000)} s`;
}

export const tokens = (n: number) => (Math.abs(n) < 1000 ? String(Math.round(n)) : `${(n / 1000).toFixed(1)}k`);

/** Signed delta using the given formatter: +25%, −1.2k, ±0. */
export const signed = (x: number, fmt: (v: number) => string) => (x === 0 ? "±0" : `${x > 0 ? "+" : "−"}${fmt(Math.abs(x))}`);

export const date = (iso: string) =>
  new Date(iso).toLocaleString("es-ES", { day: "2-digit", month: "short", hour: "2-digit", minute: "2-digit" });
