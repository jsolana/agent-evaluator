/** Horizontal proportion bar (waterfalls, distributions). `offset` and `value` are fractions of the track. */
export function Bar({ value, offset = 0, tone = "bg-indigo-500", label }: { value: number; offset?: number; tone?: string; label: string }) {
  const w = Math.max(value * 100, value > 0 ? 1.5 : 0);
  return (
    <div className="relative h-2.5 w-full rounded-full bg-slate-100" role="meter" aria-label={label} aria-valuenow={Math.round(value * 100)} aria-valuemin={0} aria-valuemax={100}>
      <div className={`absolute h-full rounded-full ${tone}`} style={{ left: `${offset * 100}%`, width: `${w}%` }} />
    </div>
  );
}
