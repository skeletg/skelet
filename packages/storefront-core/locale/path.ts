// Prefix an internal path with a locale, matching the `as-needed` scheme: the channel's default
// locale gets no prefix, everything else does
//
// Needed anywhere a path is handed to redirect()/permanentRedirect(), because those take a raw
// string and know nothing about the current language - an unprefixed target silently drops a German
// visitor back into English mid-browse
//
// Async because which locale is "default" is channel data from the backend, not a constant. The
// fetch is react-cached, so this costs nothing after the first call in a render

import { getCurrentChannelLocales } from "./read";

export async function withLocalePrefix(path: string, locale: string): Promise<string> {
  const base = path.startsWith("/") ? path : "/" + path;
  const { codes, defaultLocale } = await getCurrentChannelLocales();
  if (!locale || locale === defaultLocale) return base;
  if (codes.length > 0 && !codes.includes(locale)) return base;
  // Already prefixed - don't stack another one. The first segment ends at a query or fragment too, so /de?x=1 counts
  const first = base.split(/[/?#]/)[1] ?? "";
  if (first === locale) return base;
  // The root keeps its query without a stray slash: /?done=1 becomes /de?done=1
  return `/${locale}${base === "/" || /^\/[?#]/.test(base) ? base.slice(1) : base}`;
}
