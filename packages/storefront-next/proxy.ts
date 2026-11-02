import { print } from "graphql";
import { NextResponse, type NextRequest, type NextFetchEvent } from "next/server";

import {
  ACCESS_TOKEN_COOKIE,
  ACCESS_TOKEN_MAX_AGE,
  REFRESH_TOKEN_COOKIE,
  REFRESH_TOKEN_MAX_AGE,
  isAccessTokenExpiringSoon,
  sessionCookieOptions,
} from "@skelet/storefront-core/auth/cookies";
import {
  CustomerRefreshTokenDocument,
  type CustomerRefreshTokenMutation,
} from "@skelet/storefront-core/auth/operations.gql";
import { buildForwardedCookie } from "@skelet/storefront-core/forwarded-cookie";
import { BEACON } from "@skelet/storefront-core/beacons/paths";
import { AID_COOKIE } from "@skelet/storefront-core/analytics/visitor";
import { BACKEND_INTERNAL_URL, FRONT_API_URL } from "@skelet/storefront-core/backend-url";
import { cookieOptions } from "@skelet/storefront-core/cookie-policy";
import { pendingPromo } from "@skelet/storefront-core/promo";
import { secretEquals } from "@skelet/storefront-core/secret-equal";
import { CHANNEL_HEADER, WARM_CHANNEL_HEADER, channelForHost, isChannelId } from "@skelet/storefront-core/channel";
import { internalPath, isChannelTreePath, isLegacyMatched, isPagePath } from "@skelet/storefront-core/channel-routing";
import { clientIP } from "@skelet/storefront-core/client-ip";
import { getChannelLocales } from "@skelet/storefront-core/locale/backend-locales";
import { clearRewriteMark, isOwnRewrite, markRewrite } from "@skelet/storefront-core/rewrite-mark";

// Every request a Next storefront serves, before it renders: the channel and locale it's for, a near-expiry customer
// session rotated, the visitor id minted, a ?promo= code held for the cart, and a crawler's fetch reported. A template's
// proxy.ts calls storefrontProxy and keeps only the matcher, which Next reads from that file as written

// ?promo=CODE lands in a short-lived cookie for the next cart op to apply - middleware can't apply it itself (needs a cart and an
// authenticated gqlFetch) applyCoupon clears the cookie; the param is stripped so the address bar stays clean
const PROMO_QUERY_PARAM = "promo";

// Visitor id for Prism assignment + Lens identity. Minted here so server-side assignment has a stable id on the first paint - no variant
// flash, even for incognito Functional, not httpOnly: the client SDKs read it, LensTracker reuses the same value
const AID_OPTS = cookieOptions(60 * 60 * 24 * 365, false);

const REVALIDATE_SECRET_HEADER = "x-revalidate-secret";

const BEACON_TIMEOUT_MS = 2000;
const REFRESH_TIMEOUT_MS = 3000;

// --- Bot / crawler beacon --- Crawlers never run the Lens JS, so server-side is the only way to see what they fetch Fire-and-forget to the
// backend's /t/crawl, which re-classifies authoritatively Goes direct to the in-network backend so it can't re-enter this middleware

// Deliberately a superset of the backend's signatures - a false positive just costs a
// beacon the backend ignores, a miss costs the observation
const BOT_UA =
  /bot\b|bot\/|crawl|spider|slurp|gptbot|chatgpt|claude|anthropic|gemini|google-extended|googleother|perplexity|bingbot|adidxbot|duckduck|yandex|baidu|sogou|bytespider|ccbot|diffbot|amazonbot|applebot|facebookexternalhit|twitterbot|linkedinbot|pinterest|slackbot|telegram|whatsapp|discordbot|embedly|ahrefs|semrush|mj12|dotbot|screaming frog|blexbot|dataforseo|uptimerobot|pingdom|statuscake|datadog|lighthouse|headless|phantom|selenium|playwright|puppeteer|python-requests|python-urllib|aiohttp|scrapy|go-http-client|node-fetch|okhttp|httpclient|libwww|java\/|curl\/|wget|\+http/i;

function reportCrawl(req: NextRequest, event: NextFetchEvent, channelId: string) {
  // Wrapped so a bad env or a synchronous fetch throw can never break page serving
  try {
    const ua = req.headers.get("user-agent") ?? "";
    if (req.method !== "GET" || !ua || !BOT_UA.test(ua)) return;
    const forwardedFor = clientIP(req.headers);
    const ip = forwardedFor.split(",")[0]?.trim() ?? "";
    const ref = req.headers.get("referer") ?? "";
    const path = req.nextUrl.pathname;
    // Forward the crawler's IP so the backend rate-limits the BOT, not this Next server -
    // otherwise every beacon shares one bucket and heavy crawls drop past the cap
    const beaconHeaders: Record<string, string> = { "Content-Type": "application/json" };
    if (channelId) beaconHeaders["X-Channel-Id"] = channelId;
    if (forwardedFor) beaconHeaders["X-Forwarded-For"] = forwardedFor;
    event.waitUntil(
      fetch(`${BACKEND_INTERNAL_URL}${BEACON.crawl}`, {
        method: "POST",
        headers: beaconHeaders,
        body: JSON.stringify({ path, ua, ip, ref }),
        // Bounded so a slow backend can't leave sockets hanging on a high-volume crawl
        signal: AbortSignal.timeout(BEACON_TIMEOUT_MS),
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
const REFRESH_MUTATION = print(CustomerRefreshTokenDocument);

type RefreshOutcome =
  | { kind: "noop" }
  | { kind: "refreshed"; accessToken: string; refreshToken: string }
  | { kind: "dead" };

async function maybeRefreshSession(req: NextRequest, channelId: string): Promise<RefreshOutcome> {
  const refresh = req.cookies.get(REFRESH_TOKEN_COOKIE)?.value;
  if (!refresh) return { kind: "noop" }; // anonymous - nothing to refresh
  const access = req.cookies.get(ACCESS_TOKEN_COOKIE)?.value;
  if (!isAccessTokenExpiringSoon(access)) return { kind: "noop" }; // still fresh
  try {
    const headers: Record<string, string> = { "Content-Type": "application/json", "X-Channel-Id": channelId };
    const forwardedFor = clientIP(req.headers);
    if (forwardedFor) headers["X-Forwarded-For"] = forwardedFor;
    const r = await fetch(FRONT_API_URL, {
      method: "POST",
      headers,
      body: JSON.stringify({ query: REFRESH_MUTATION, variables: { token: refresh } }),
      signal: AbortSignal.timeout(REFRESH_TIMEOUT_MS),
      cache: "no-store",
    });
    // 5xx / network errors are transient - keep the session and try next request.
    if (!r.ok) return { kind: "noop" };
    const body = (await r.json()) as {
      data?: CustomerRefreshTokenMutation;
      errors?: { extensions?: { class?: unknown } }[];
    };
    const p = body?.data?.customerRefreshToken;
    if (p?.accessToken && p?.refreshToken) {
      return { kind: "refreshed", accessToken: p.accessToken, refreshToken: p.refreshToken };
    }
    // Only the backend calling the token no good ends the session - a rate limit or an outage keeps it for the next try
    if (body?.errors?.some((e) => e.extensions?.class === "UNAUTHENTICATED")) return { kind: "dead" };
    return { kind: "noop" };
  } catch {
    // Timeout / network hiccup - transient, keep the session.
    return { kind: "noop" };
  }
}

// --- Locale routing ---
// The channel's locale set comes from the backend (the core's locale/backend-locales), shared with the render path so
// there is exactly one place that knows which languages exist
// A rewrite, not a redirect: the /<channel>/<locale>/ path is the cache key and the browser keeps the clean url
async function channelRewrite(req: NextRequest, channelId: string): Promise<URL | null> {
  const { codes, defaultLocale } = await getChannelLocales(channelId);
  const to = internalPath(req.nextUrl.pathname, channelId, codes, defaultLocale);
  if (!to) return null;
  const url = req.nextUrl.clone();
  url.pathname = to;
  return url;
}

// The host decides the channel, except for the revalidate route's own warm requests, which name the channel they rebuild
function requestChannel(req: NextRequest): string {
  const warm = req.headers.get(WARM_CHANNEL_HEADER);
  if (isChannelId(warm) && secretEquals(req.headers.get(REVALIDATE_SECRET_HEADER), process.env.STOREFRONT_REVALIDATE_SECRET ?? "")) {
    return warm.toLowerCase();
  }
  return channelForHost(req.headers.get("host") ?? "");
}

function forwardedHeaders(req: NextRequest, channelId: string): Headers {
  const headers = new Headers(req.headers);
  headers.delete(WARM_CHANNEL_HEADER);
  headers.delete(REVALIDATE_SECRET_HEADER);
  if (channelId) headers.set(CHANNEL_HEADER, channelId);
  else headers.delete(CHANNEL_HEADER);
  return headers;
}

export async function storefrontProxy(req: NextRequest, event: NextFetchEvent): Promise<NextResponse> {
  const pathname = req.nextUrl.pathname;
  // The /<channel>/ tree is reached only through the rewrite below. Production runs the proxy again on that rewrite,
  // and that pass - marked by the first - goes straight through: a second session refresh or crawl report would be wrong
  if (isChannelTreePath(pathname)) {
    if (!isOwnRewrite(req.headers)) return new NextResponse("Not found", { status: 404 });
    const headers = new Headers(req.headers);
    clearRewriteMark(headers);
    return NextResponse.next({ request: { headers } });
  }
  const channelId = requestChannel(req);

  // Route handlers and files only need the channel stamped; one arriving on a host no channel claims passes through bare
  if (!isLegacyMatched(pathname)) {
    const headers = forwardedHeaders(req, channelId);
    clearRewriteMark(headers);
    return NextResponse.next({ request: { headers } });
  }
  if (!channelId && isPagePath(pathname)) {
    return new NextResponse("No storefront is configured for this host", { status: 404 });
  }

  // Record crawler page fetches (runs for every matched page request).
  reportCrawl(req, event, channelId);

  const url = req.nextUrl;
  const code = url.searchParams.get(PROMO_QUERY_PARAM);

  // Proactively rotate a near-expiry customer session before render.
  const session = channelId ? await maybeRefreshSession(req, channelId) : ({ kind: "noop" } as const);

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

  if (code !== null) {
    // Strip the param so reloads and shares don't re-fire the code
    const cleaned = url.clone();
    cleaned.searchParams.delete(PROMO_QUERY_PARAM);
    const res = NextResponse.redirect(cleaned);
    const held = pendingPromo(code);
    if (held) res.cookies.set(...held);
    return applyCookies(res);
  }

  // Overridden on the forwarded request so THIS render sees them - new visitor id for
  // first-paint assignment, rotated access token for its gqlFetch calls
  const forward: Record<string, string> = {};
  if (newAid) forward[AID_COOKIE] = newAid;
  if (session.kind === "refreshed") {
    forward[ACCESS_TOKEN_COOKIE] = session.accessToken;
    forward[REFRESH_TOKEN_COOKIE] = session.refreshToken;
  }

  const rewriteTo = channelId ? await channelRewrite(req, channelId) : null;

  const headers = forwardedHeaders(req, channelId);
  if (rewriteTo) markRewrite(headers);
  else clearRewriteMark(headers);
  if (Object.keys(forward).length > 0) {
    headers.set("cookie", buildForwardedCookie(headers.get("cookie"), forward));
  }
  return applyCookies(
    rewriteTo
      ? NextResponse.rewrite(rewriteTo, { request: { headers } })
      : NextResponse.next({ request: { headers } }),
  );
}
