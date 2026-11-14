import type { UseQueryResult } from "@tanstack/react-query";
import type { ReactNode } from "react";

/** Uniform loading / error handling so pages only render the happy path. */
export function QueryState<T>({ query, children }: { query: UseQueryResult<T>; children: (data: T) => ReactNode }) {
  if (query.isPending) return <div aria-busy="true" className="h-40 animate-pulse rounded-2xl bg-slate-200/60" />;
  if (query.isError)
    return <div role="alert" className="rounded-2xl bg-rose-50 p-4 text-sm text-rose-700 ring-1 ring-rose-200">{query.error.message}</div>;
  return <>{children(query.data)}</>;
}
