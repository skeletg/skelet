// Server-side presentment loader: the available display currencies for the current store (active +
// fresh ECB rate, resolved by the backend) - deliberately NO cookie read: the visitor's chosen
// currency is applied client-side by PresentmentProvider, so a cached page carries the same options
// for everyone

import { type PresentmentCurrency } from "@/app/components/PresentmentProvider";
import { PresentmentCurrenciesDocument } from "@/app/lib/presentment/operations.gql";
import { gqlFetch } from "@/app/lib/graphql";
import { surfaceCacheOpts, NAV_SURFACE, NAV_TAG } from "@/app/lib/config/read";

export async function getPresentmentOptions(): Promise<{ options: PresentmentCurrency[] }> {
  let options: PresentmentCurrency[] = [];
  try {
    // Store display currencies + rates: identical for every visitor and refreshed on a
    // schedule, so cacheable under the shared "nav" surface
    const cacheOpts = await surfaceCacheOpts(NAV_SURFACE, { tags: [NAV_TAG] });
    const res = await gqlFetch(PresentmentCurrenciesDocument, {}, cacheOpts);
    options = (res.data.presentmentCurrencies ?? []).map((c) => ({
      code: c.code,
      name: c.name,
      symbol: c.symbol,
      decimals: c.decimals,
      symbolPosition: c.symbolPosition,
      factorFromBase: c.factorFromBase,
    }));
  } catch {
    // Rates unavailable - the switcher simply doesn't render
  }
  return { options };
}
