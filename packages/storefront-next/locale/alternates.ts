// Canonical + hreflang for a page that exists in every locale this channel serves
//
// Both matter and they pull opposite ways: without the prefix a German page canonicalises to the
// English url and search engines drop it as a duplicate, which is the exact opposite of why the
// locale went into the url. `languages` then tells them the two are translations, not rivals

import { getLocale } from "next-intl/server";

import { getCurrentChannelLocales } from "./read";

function withLocale(path: string, locale: string, defaultLocale: string): string {
  // Keep any query on the end - a paginated canonical is `${href}?page=2`
  const [p, q] = path.split("?");
  const base = p || "/";
  const prefixed = locale === defaultLocale ? base : `/${locale}${base === "/" ? "" : base}`;
  return q ? `${prefixed}?${q}` : prefixed;
}

// For URLs embedded in page content (JSON-LD, feeds) rather than <head> alternates: resolves the
// request locale once and returns a sync path -> locale-canonical path mapper, so a page building
// many URLs (a breadcrumb trail, an item list) pays the lookup once
export async function canonicalPathMapper(): Promise<(path: string) => string> {
  const [locale, { defaultLocale }] = await Promise.all([getLocale(), getCurrentChannelLocales()]);
  return (path) => withLocale(path, locale, defaultLocale);
}

export async function localizedAlternates(path: string): Promise<{
  canonical: string;
  languages: Record<string, string>;
}> {
  const [locale, { codes, defaultLocale, hreflang }] = await Promise.all([
    getLocale(),
    getCurrentChannelLocales(),
  ]);
  const languages: Record<string, string> = {};
  // Keyed by the backend's hreflang value, not our internal code - "en" and "en-GB" are one locale
  // to the catalog and two different things to a search engine
  for (const c of codes) languages[hreflang[c] ?? c] = withLocale(path, c, defaultLocale);
  if (defaultLocale) {
    // Which url an engine should serve when it has no better locale match
    languages["x-default"] = withLocale(path, defaultLocale, defaultLocale);
  }
  return { canonical: withLocale(path, locale, defaultLocale), languages };
}
