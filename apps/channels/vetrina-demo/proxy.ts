import { NextResponse, type NextRequest, type NextFetchEvent } from "next/server";

import {
  ACCESS_TOKEN_COOKIE,
  ACCESS_TOKEN_MAX_AGE,
  REFRESH_TOKEN_COOKIE,
  REFRESH_TOKEN_MAX_AGE,
  isAccessTokenExpiringSoon,
  sessionCookieOptions,
} from "@/app/lib/auth/cookies";
import { buildForwardedCookie } from "@/app/lib/forwarded-cookie";


// Mirrors the backend's FRONT_API_SLUG - empty keeps /api/front/graphql, so this is inert unless set
const FRONT_API_PATH = process.env.FRONT_API_SLUG
  ? `/api/front/${process.env.FRONT_API_SLUG}/graphql`
  : "/api/front/graphql";

// ?promo=CODE lands in a short-lived cookie for the next cart op to apply - middleware can't apply it itself (needs a cart and an
// authenticated gqlFetch) applyCoupon clears the cookie; the param is stripped so the address bar stays clean
const PROMO_QUERY_PARAM = "promo";
export const PROMO_PENDING_COOKIE = "storefront_pending_promo";

// Visitor id for Prism assignment + Lens identity. Minted here so server-side assignment has a stable id on the first paint - no variant
// flash, even for incognito Functional, not httpOnly: the client SDKs read it, LensTracker reuses the same value
const AID_COOKIE = "lens_aid";
const AID_OPTS = {
  path: "/",
  httpOnly: false,
  sameSite: "lax" as const,
  maxAge: 60 * 60 * 24 * 365,
};

// --- Bot / crawler beacon --- Crawlers never run the Lens JS, so server-side is the only way to see what they fetch Fire-and-forget to the
// backend's /t/crawl, which re-classifies authoritatively Goes direct to the in-network backend so it can't re-enter this middleware
// Channel uuid, not a slug - unset means the backend resolves via Host or its default
const DEFAULT_CHANNEL_ID = requireChannelId();
const CHANNEL_COOKIE = "storefront_channel";

// Deliberately a superset of the backend's signatures - a false positive just costs a
// beacon the backend ignores, a miss costs the observation
const BOT_UA =
  /bot\b|bot\/|crawl|spider|slurp|gptbot|chatgpt|claude|anthropic|gemini|google-extended|googleother|perplexity|bingbot|adidxbot|duckduck|yandex|baidu|sogou|bytespider|ccbot|diffbot|amazonbot|applebot|facebookexternalhit|twitterbot|linkedinbot|pinterest|slackbot|telegram|whatsapp|discordbot|embedly|ahrefs|semrush|mj12|dotbot|screaming frog|blexbot|dataforseo|uptimerobot|pingdom|statuscake|datadog|lighthouse|headless|phantom|selenium|playwright|puppeteer|python-requests|python-urllib|aiohttp|scrapy|go-http-client|node-fetch|okhttp|httpclient|libwww|java\/|curl\/|wget|\+http/i;

function reportCrawl(req: NextRequest, event: NextFetchEvent) {
  // Wrapped so a bad env or a synchronous fetch throw can never break page serving
  try {
    const ua = req.headers.get("user-agent") ?? "";
    if (req.method !== "GET" || !ua || !BOT_UA.test(ua)) return;
    const channelId = req.cookies.get(CHANNEL_COOKIE)?.value ?? DEFAULT_CHANNEL_ID;
    const ip =
      (req.headers.get("x-forwarded-for") ?? "").split(",")[0]?.trim() ||
      (req.headers.get("x-real-ip") ?? "");
    const ref = req.headers.get("referer") ?? "";
    const path = req.nextUrl.pathname;
    // Forward the crawler's IP so the backend rate-limits the BOT, not this Next server -
    // otherwise every beacon shares one bucket and heavy crawls drop past the cap
    const beaconHeaders: Record<string, string> = { "Content-Type": "application/json" };
    if (channelId) beaconHeaders["X-Channel-Id"] = channelId;
    if (ip) beaconHeaders["X-Forwarded-For"] = ip;
    event.waitUntil(
      fetch(`${BACKEND_INTERNAL_URL}/t/crawl`, {
        method: "POST",
        headers: beaconHeaders,
        body: JSON.stringify({ path, ua, ip, ref }),
        // Bounded so a slow backend can't leave sockets hanging on a high-volume crawl
        signal: AbortSignal.timeout(2000),
      }).catch(() => {
        // a tracking hiccup must never affect the crawler's response
      }),
    );
  } catch {
    // swallow - analytics must never break the response path
  }
}

// --- Session refresh ---
// Refresh MUST be here, not in gqlFetch - that runs during RSC render where Next forbids
// writing cookies, so it would rotate backend-side and fail to persist, burning the
// session. Proactive, so a lost race is harmless and one refresh covers every fetch
const REFRESH_MUTATION =
  "mutation MiddlewareRefresh($token: String!) { customerRefreshToken(token: $token) { accessToken refreshToken } }";

type RefreshOutcome =
  | { kind: "noop" }
  | { kind: "refreshed"; accessToken: string; refreshToken: string }
  | { kind: "dead" };

async function maybeRefreshSession(req: NextRequest): Promise<RefreshOutcome> {
  const refresh = req.cookies.get(REFRESH_TOKEN_COOKIE)?.value;
  if (!refresh) return { kind: "noop" }; // anonymous - nothing to refresh
  const access = req.cookies.get(ACCESS_TOKEN_COOKIE)?.value;
  if (!isAccessTokenExpiringSoon(access)) return { kind: "noop" }; // still fresh
  const channelId = req.cookies.get(CHANNEL_COOKIE)?.value ?? DEFAULT_CHANNEL_ID;
  try {
    const headers: Record<string, string> = { "Content-Type": "application/json" };
    if (channelId) headers["X-Channel-Id"] = channelId;
    const r = await fetch(`${BACKEND_INTERNAL_URL}${FRONT_API_PATH}`, {
      method: "POST",
      headers,
      body: JSON.stringify({ query: REFRESH_MUTATION, variables: { token: refresh } }),
      signal: AbortSignal.timeout(3000),
      cache: "no-store",
    });
    // 5xx / network errors are transient - keep the session and try next request.
    if (!r.ok) return { kind: "noop" };
    const body = (await r.json()) as {
      data?: { customerRefreshToken?: { accessToken?: string; refreshToken?: string } };
    };
    const p = body?.data?.customerRefreshToken;
    if (p?.accessToken && p?.refreshToken) {
      return { kind: "refreshed", accessToken: p.accessToken, refreshToken: p.refreshToken };
    }
    // No tokens back means a genuinely dead refresh token - the backend's rotation grace
    // window means a concurrent prefetch race lands elsewhere. Safe to clear
    return { kind: "dead" };
  } catch {
    // Timeout / network hiccup - transient, keep the session.
    return { kind: "noop" };
  }
}

// --- Locale routing ---
// The channel's locale set comes from the backend (app/lib/locale/backend-locales), shared with the
// render path so there is exactly one place that knows which languages exist
import { getChannelLocales } from "@/app/lib/locale/backend-locales";
import { BACKEND_INTERNAL_URL } from "@/app/lib/backend-url";
import { requireChannelId } from "@/app/lib/channel";

// Rewrites an unprefixed path onto the default locale segment, so the app tree always sees
// /<locale>/... while the browser keeps the clean url. A REWRITE, not a redirect: the rewritten
// path is the cache key, so /x and /de/x stay two separate cached documents
// First segments that are NOT pages and must keep their exact path: the backend proxies declared in
// next.config (those rewrites run after this one, so a locale prefix would stop them matching) and
// the route handlers that stayed at the app root because external links point straight at them
const NOT_LOCALIZED = new Set(["api", "t", "download", "unsubscribe", "e", "coupon", "masquerade"]);

async function localeRewrite(req: NextRequest): Promise<URL | null> {
  const { pathname } = req.nextUrl;
  const first = pathname.split("/")[1] ?? "";
  if (NOT_LOCALIZED.has(first)) return null;
  const { codes, defaultLocale } = await getChannelLocales();
  if (!defaultLocale) return null; // unknown yet - let the app render its default rather than guess a prefix
  if (codes.includes(first)) return null; // already prefixed
  const url = req.nextUrl.clone();
  url.pathname = `/${defaultLocale}${pathname === "/" ? "" : pathname}`;
  return url;
}

export async function proxy(req: NextRequest, event: NextFetchEvent) {
  // Record crawler page fetches (runs for every matched page request).
  reportCrawl(req, event);

  const url = req.nextUrl;
  const code = url.searchParams.get(PROMO_QUERY_PARAM)?.trim();

  // Proactively rotate a near-expiry customer session before render.
  const session = await maybeRefreshSession(req);

  const existingAid = req.cookies.get(AID_COOKIE)?.value;
  const newAid = existingAid ? undefined : crypto.randomUUID();

  // Writes applied to whatever response we return (persist to the browser).
  const applyCookies = (res: NextResponse) => {
    // Re-set even when present: rolls the 1-year expiry per visit, and upgrades a tracker-minted cookie
    // (script-set, so ITP-capped at 7 days on Safari) to a server-set one that actually keeps its expiry
    res.cookies.set(AID_COOKIE, newAid ?? existingAid!, AID_OPTS);
    if (session.kind === "refreshed") {
      const opts = sessionCookieOptions();
      res.cookies.set(ACCESS_TOKEN_COOKIE, session.accessToken, { ...opts, maxAge: ACCESS_TOKEN_MAX_AGE });
      res.cookies.set(REFRESH_TOKEN_COOKIE, session.refreshToken, { ...opts, maxAge: REFRESH_TOKEN_MAX_AGE });
    } else if (session.kind === "dead") {
      // Clear both so we stop retrying and the UI renders logged-out cleanly
      const opts = sessionCookieOptions();
      res.cookies.set(ACCESS_TOKEN_COOKIE, "", { ...opts, maxAge: 0 });
      res.cookies.set(REFRESH_TOKEN_COOKIE, "", { ...opts, maxAge: 0 });
    }
    return res;
  };

  if (code) {
    // Strip the param so reloads and shares don't re-fire the code
    const cleaned = url.clone();
    cleaned.searchParams.delete(PROMO_QUERY_PARAM);
    const res = NextResponse.redirect(cleaned);
    res.cookies.set(PROMO_PENDING_COOKIE, code.toUpperCase(), {
      path: "/",
      httpOnly: false,
      sameSite: "lax",
      maxAge: 60 * 60 * 24 * 7, // 7 days
    });
    return applyCookies(res);
  }

  // Overridden on the forwarded request so THIS render sees them - new visitor id for
  // first-paint assignment, rotated access token for its gqlFetch calls
  const forward: Record<string, string> = {};
  if (newAid) forward[AID_COOKIE] = newAid;
  if (session.kind === "refreshed") forward[ACCESS_TOKEN_COOKIE] = session.accessToken;

  const rewriteTo = await localeRewrite(req);

  if (Object.keys(forward).length > 0) {
    const headers = new Headers(req.headers);
    headers.set("cookie", buildForwardedCookie(headers.get("cookie"), forward));
    return applyCookies(
      rewriteTo
        ? NextResponse.rewrite(rewriteTo, { request: { headers } })
        : NextResponse.next({ request: { headers } }),
    );
  }
  return applyCookies(rewriteTo ? NextResponse.rewrite(rewriteTo) : NextResponse.next());
}

export const config = {
  // Every customer-facing route - promo pickup and session refresh both want the lot
  matcher: ["/((?!_next|api|.*\\.).*)"],
};
