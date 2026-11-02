import type { BrowseProductsQueryVariables } from "./operations.gql";

const VARS_WITH_DEFAULTS = new Set(["limit", "offset"]);

export function normalizeBrowseVars(vars: BrowseProductsQueryVariables): BrowseProductsQueryVariables {
  const out: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(vars)) {
    if (v === null || v === undefined) {
      if (VARS_WITH_DEFAULTS.has(k)) out[k] = v;
      continue;
    }
    if (k === "filter" && typeof v === "object") {
      const filter = Object.fromEntries(
        Object.entries(v as Record<string, unknown>).filter(([, fv]) => fv !== null && fv !== undefined),
      );
      if (Object.keys(filter).length > 0) out[k] = filter;
      continue;
    }
    out[k] = v;
  }
  return out as BrowseProductsQueryVariables;
}
