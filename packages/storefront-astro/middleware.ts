// The storefront core's questions about a request, answered the Astro way. Astro has no request scope a module can
// reach, so this middleware opens one: every core call a page, an endpoint or a component makes while the request
// renders finds it through AsyncLocalStorage. src/middleware.ts re-exports it as onRequest

import { AsyncLocalStorage } from "node:async_hooks";

import type { APIContext, MiddlewareHandler } from "astro";

import { channelForHost } from "@skelet/storefront-core/channel";
import { configureStorefront, type CookieJar, type StorefrontRequest } from "@skelet/storefront-core/request";

import { cachedFetch } from "./data-cache";

// The core's exits, thrown through the page and turned into a response here
class Redirect extends Error {
  constructor(readonly to: string) {
    super(`redirect to ${to}`);
  }
}

class NotFound extends Error {
  constructor() {
    super("not found");
  }
}

type Scope = { request: StorefrontRequest; memo: Map<unknown, Map<string, unknown>> };

const scope = new AsyncLocalStorage<Scope>();

configureStorefront({
  request: () => {
    const s = scope.getStore();
    if (!s) {
      throw new Error("No storefront request in scope - src/middleware.ts must export storefront from @skelet/storefront-astro/middleware");
    }
    return s.request;
  },
  // Memoized for the request the call runs in; outside one - a build step, a script - nothing is kept
  cache:
    <A extends unknown[], R>(fn: (...args: A) => R) =>
    (...args: A): R => {
      const s = scope.getStore();
      if (!s) return fn(...args);
      let byArgs = s.memo.get(fn);
      if (!byArgs) s.memo.set(fn, (byArgs = new Map()));
      const key = JSON.stringify(args);
      if (!byArgs.has(key)) byArgs.set(key, fn(...args));
      return byArgs.get(key) as R;
    },
});

function astroRequest(context: APIContext): StorefrontRequest {
  const jar: CookieJar = {
    get: (name) => {
      const c = context.cookies.get(name);
      return c ? { value: c.value } : undefined;
    },
    set: (name, value, options) => context.cookies.set(name, value, { path: "/", ...options }),
    delete: (name) => context.cookies.delete(name, { path: "/" }),
  };
  return {
    cookies: async () => jar,
    headers: async () => context.request.headers,
    // Empty without Astro's i18n routing - the backend answers in the channel's default locale then
    locale: async () => context.currentLocale ?? "",
    // A host mapped in STOREFRONT_CHANNEL_HOSTS, else STOREFRONT_CHANNEL_ID - the same rule as the Next proxy
    channelId: async () => channelForHost(context.request.headers.get("host") ?? ""),
    fetch: (url, init, hint) => (hint ? cachedFetch(url, init, hint) : fetch(url, init)),
    redirect: (to) => {
      throw new Redirect(to);
    },
    notFound: () => {
      throw new NotFound();
    },
    // Astro's route cache is the template's, keyed by tags it sets itself - the core's paths name Next's routes
    revalidate: () => {},
    // A plain form post already renders the page afresh - post, redirect, get
    refresh: () => {},
  };
}

export const storefront: MiddlewareHandler = (context, next) =>
  scope.run({ request: astroRequest(context), memo: new Map() }, async () => {
    try {
      return await next();
    } catch (err) {
      if (err instanceof Redirect) return context.redirect(err.to, 303);
      if (err instanceof NotFound) return context.rewrite("/404");
      throw err;
    }
  });
