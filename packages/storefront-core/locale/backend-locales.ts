import { print } from "graphql";

import { FRONT_API_URL } from "../backend-url";
import { ChannelLocalesDocument, type ChannelLocalesQuery } from "./operations.gql";
// A channel's locale set, straight from the backend. Single source for the whole app -
// the proxy, the next-intl request config, and render-time callers all read this
//
// A PLAIN fetch, deliberately not gqlFetch: gqlFetch asks next-intl for the current locale, and
// next-intl's request config needs this answer to know what the locale may be. Going through it
// would be a cycle. This also means it works in the proxy, which has no request scope at all
//
// Module-level TTL cache rather than react cache() for the same reason - it has to work outside a
// render. One request a minute per channel per process

// Printed once - the proxy calls this on every request
const QUERY = print(ChannelLocalesDocument);
const TTL_MS = 60_000;

export type ChannelLocales = {
  codes: string[];
  defaultLocale: string;
  // Locale code -> the value for hreflang, which the backend owns because "en" and "en-GB" are one
  // locale to the catalog and two different things to a search engine
  hreflang: Record<string, string>;
};

const EMPTY: ChannelLocales = { codes: [], defaultLocale: "", hreflang: {} };
const cache = new Map<string, { at: number; value: ChannelLocales }>();

export async function getChannelLocales(channelId: string): Promise<ChannelLocales> {
  if (!channelId) return EMPTY; // without a channel the backend would answer for its default one
  const cached = cache.get(channelId);
  if (cached && Date.now() - cached.at < TTL_MS) return cached.value;
  try {
    const headers: Record<string, string> = { "Content-Type": "application/json", "X-Channel-Id": channelId };
    const r = await fetch(FRONT_API_URL, {
      method: "POST",
      headers,
      body: JSON.stringify({ query: QUERY }),
      signal: AbortSignal.timeout(2000),
      cache: "no-store",
    });
    if (!r.ok) throw new Error(String(r.status));
    const body = (await r.json()) as { data?: ChannelLocalesQuery };
    const list = body.data?.currentChannel?.locales ?? [];
    const codes = list.map((l) => l.code);
    const hreflang: Record<string, string> = {};
    for (const l of list) hreflang[l.code] = l.hreflangValue || l.code;
    const value: ChannelLocales = {
      codes,
      defaultLocale: body.data?.currentChannel?.defaultLocale ?? codes[0] ?? "",
      hreflang,
    };
    cache.set(channelId, { at: Date.now(), value });
    return value;
  } catch {
    // Keep serving the last good answer. With none, an empty set means callers fall back to doing
    // nothing (no prefix, no 404, no hreflang) rather than guessing a language
    return cached?.value ?? EMPTY;
  }
}
