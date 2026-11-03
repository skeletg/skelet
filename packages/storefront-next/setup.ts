// The storefront core's questions about a request, answered the Next way. Every module here that reaches the core's
// reads or actions imports this first - a server bundle layer keeps its own copy of the core, so each configures its own

import { refresh, revalidatePath } from "next/cache";
import { cookies, headers } from "next/headers";
import { notFound, redirect, RedirectType, unstable_rethrow } from "next/navigation";
import { channel } from "next/root-params";
import { getLocale } from "next-intl/server";
import { cache } from "react";

import { CHANNEL_HEADER, PINNED_CHANNEL_ID } from "@skelet/storefront-core/channel";
import { channelRoutePaths } from "@skelet/storefront-core/channel-routing";
import { getCurrentChannelId, getCurrentChannelLocales, getCurrentLocale } from "@skelet/storefront-core/locale/read";
import { configureStorefront, type StorefrontRequest } from "@skelet/storefront-core/request";

const next: StorefrontRequest = {
  cookies: () => cookies(),
  headers: () => headers(),
  // From the [locale] route segment, never from a cookie. A cookie can't be read on the routes that serve most of the
  // site (they render without a request), so the segment is part of the cache key instead, one cached page per language
  locale: () => getLocale(),
  // Pages read the [channel] segment, so static renders are per channel; route handlers and actions read the proxy's
  // header
  async channelId() {
    try {
      const c = await channel();
      if (c) return c;
    } catch (err) {
      unstable_rethrow(err);
    }
    try {
      const c = (await headers()).get(CHANNEL_HEADER);
      if (c) return c;
    } catch (err) {
      unstable_rethrow(err);
    }
    return PINNED_CHANNEL_ID;
  },
  fetch: (url, init, hint) =>
    fetch(url, hint ? { ...init, cache: "force-cache", next: hint } : { ...init, cache: "no-store" }),
  // No mode keeps Next's own default for where the redirect happens - push from an action, replace elsewhere
  redirect: (to, mode) => (mode ? redirect(to, mode === "replace" ? RedirectType.replace : RedirectType.push) : redirect(to)),
  notFound: () => notFound(),
  // Next caches by the route a page renders from - the proxy's /<channel>/<locale>/ rewrite, in every locale
  async revalidatePublic(path) {
    const [channelId, { codes }] = await Promise.all([getCurrentChannelId(), getCurrentChannelLocales()]);
    const locales = codes.length > 0 ? codes : [await getCurrentLocale()];
    for (const route of channelRoutePaths(path, channelId, locales)) revalidatePath(route);
  },
  revalidateAccount: () => revalidatePath("/[channel]/[locale]/account", "layout"),
  refresh: () => refresh(),
};

configureStorefront({ request: () => next, cache, rethrowControlFlow: unstable_rethrow });
