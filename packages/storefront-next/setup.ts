// The storefront core's questions about a request, answered the Next way. Every module here that reaches the core's
// reads or actions imports this first - a server bundle layer keeps its own copy of the core, so each configures its own

import { refresh, revalidatePath } from "next/cache";
import { cookies, headers } from "next/headers";
import { notFound, redirect, RedirectType } from "next/navigation";
import { channel } from "next/root-params";
import { getLocale } from "next-intl/server";
import { cache } from "react";

import { CHANNEL_HEADER, PINNED_CHANNEL_ID } from "@skelet/storefront-core/channel";
import { rethrowIfDynamicBailout } from "@skelet/storefront-core/dynamic-bailout";
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
    } catch {
      // not under the [channel] root layout
    }
    try {
      const c = (await headers()).get(CHANNEL_HEADER);
      if (c) return c;
    } catch (err) {
      rethrowIfDynamicBailout(err);
    }
    return PINNED_CHANNEL_ID;
  },
  fetch: (url, init, hint) =>
    fetch(url, hint ? { ...init, cache: "force-cache", next: hint } : { ...init, cache: "no-store" }),
  // No mode keeps Next's own default for where the redirect happens - push from an action, replace elsewhere
  redirect: (to, mode) => (mode ? redirect(to, mode === "replace" ? RedirectType.replace : RedirectType.push) : redirect(to)),
  notFound: () => notFound(),
  revalidate: (path, scope) => (scope ? revalidatePath(path, scope) : revalidatePath(path)),
  refresh: () => refresh(),
};

configureStorefront({ request: () => next, cache });
