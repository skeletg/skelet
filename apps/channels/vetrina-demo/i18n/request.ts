import { getRequestConfig } from "next-intl/server";

import { getChannelLocales } from "@/app/lib/locale/backend-locales";

// Per-request i18n config. `locale` comes from the [locale] route segment, which means it is part
// of the static cache key - the whole point of the move: a cached page can be per-language, where a
// cookie read could never be
//
// No locale list here on purpose. Which locales exist is channel data the backend owns, and the
// layout rejects anything the channel doesn't serve. The fallback is for the handful of routes that
// live OUTSIDE [locale] - sitemap, robots - where there is no segment to read
export default getRequestConfig(async ({ requestLocale }) => {
  const requested = await requestLocale;
  const locale = requested || (await getChannelLocales()).defaultLocale;
  return {
    // Empty would mean the backend never got a Locale header at all, and it clamps to the channel
    // default anyway - but next-intl needs a string, so pass through whatever we resolved
    locale,
    // UI strings only. Product names, category names and CMS content are localized by the backend
    // off the `Locale` header, not from these files
    messages: (await import(`@/messages/${locale}.json`).catch(() => ({ default: {} }))).default,
  };
});
