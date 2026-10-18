import { getRequestConfig } from "next-intl/server";

import { getCurrentChannelLocales } from "@/app/lib/locale/read";
import en from "@/messages/en.json";

// Per-request i18n config. `locale` comes from the [locale] route segment, which means it is part
// of the static cache key - the whole point of the move: a cached page can be per-language, where a
// cookie read could never be
//
// No locale list here on purpose. Which locales exist is channel data the backend owns, and the
// layout rejects anything the channel doesn't serve. The fallback is for the handful of routes that
// live OUTSIDE [locale] - sitemap, robots - where there is no segment to read
export default getRequestConfig(async ({ requestLocale }) => {
  const requested = await requestLocale;
  const locale = requested || (await getCurrentChannelLocales()).defaultLocale;
  return {
    // Empty would mean the backend never got a Locale header at all, and it clamps to the channel
    // default anyway - but next-intl needs a string, so pass through whatever we resolved
    locale,
    // UI strings only. Product names, category names and CMS content are localized by the backend
    // off the `Locale` header, not from these files
    messages: deepMerge(en, await loadMessages(locale)),
  };
});

// English is the base every locale sits on, so a key a translation hasn't reached yet renders in English instead of
// throwing MISSING_MESSAGE
async function loadMessages(locale: string): Promise<Messages> {
  if (locale === "en") return {};
  return (await import(`@/messages/${locale}.json`).catch(() => ({ default: {} }))).default;
}

type Messages = { [key: string]: string | Messages };

function deepMerge(base: Messages, over: Messages): Messages {
  const out: Messages = { ...base };
  for (const [k, v] of Object.entries(over)) {
    const b = out[k];
    out[k] = typeof v === "object" && typeof b === "object" ? deepMerge(b, v) : v;
  }
  return out;
}
