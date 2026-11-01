import { print } from "graphql";
import { NextResponse, type NextRequest, type NextFetchEvent } from "next/server";

import {
  ACCESS_TOKEN_COOKIE,
  ACCESS_TOKEN_MAX_AGE,
  REFRESH_TOKEN_COOKIE,
  REFRESH_TOKEN_MAX_AGE,
  isAccessTokenExpiringSoon,
  sessionCookieOptions,
} from "@/app/lib/auth/cookies";
import {
  CustomerRefreshTokenDocument,
  type CustomerRefreshTokenMutation,
} from "@/app/lib/auth/operations.gql";
import { buildForwardedCookie } from "@/app/lib/forwarded-cookie";
import { BEACON } from "@/app/lib/beacons/paths";
import { clientIP } from "@/app/lib/client-ip";
import { CONSENT_COOKIE, CONSENT_COOKIE_OPTS } from "@/app/lib/consent/cookie";
import { parseConsent, resolveConsentCookie } from "@/app/lib/consent/categories";
import { getConsentMode } from "@/app/lib/consent/mode";
import { clickCookieWrites } from "@/app/lib/consent/click-ids";


// Mirrors the backend's FRONT_API_SLUG - empty keeps /api/front/graphql, so this is inert unless set
const FRONT_API_PATH = process.env.FRONT_API_SLUG
  ? `/api/front/${process.env.FRONT_API_SLUG}/graphql`
  : "/api/front/graphql";

// ?promo=CODE lands in a short-lived cookie for the next cart op to apply - middleware can't apply it itself (needs a cart and an
// authenticated gqlFetch) applyCoupon clears the cookie; the param is stripped so the address bar stays clean
const PROMO_QUERY_PARAM = "promo";
export const PROMO_PENDING_COOKIE = "storefront_pending_promo";

// Visitor id for Prism assignment + Lens identity. Minted here so server-side assignment has a stable id on the first paint - no variant
// flash, even for incognito. Only with analytics consent: without it there is no id, experiments render their control, and a
// withdrawn grant deletes the cookie on the next request. Not httpOnly: the client SDKs read it, LensTracker reuses the same value
const AID_COOKIE = "lens_aid";
const AID_OPTS = {
  path: "/",
  httpOnly: false,
  sameSite: "lax" as const,
  maxAge: 60 * 60 * 24 * 365,
};

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
    const r = await fetch(`${BACKEND_INTERNAL_URL}${FRONT_API_PATH}`, {
      method: "POST",
      headers,
      body: JSON.stringify({ query: REFRESH_MUTATION, variables: { token: refresh } }),
      signal: AbortSignal.timeout(3000),
      cache: "no-store",
    });
    // 5xx / network errors are transient - keep the session and try next request.
    if (!r.ok) return { kind: "noop" };
    const body = (await r.json()) as { data?: CustomerRefreshTokenMutation };
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
import { CHANNEL_HEADER, WARM_CHANNEL_HEADER, channelForHost, isChannelId } from "@/app/lib/channel";
import { internalPath, isChannelTreePath, isLegacyMatched, isPagePath } from "@/app/lib/channel-routing";

// A rewrite, not a redirect: the /<channel>/<locale>/ path is the cache key and the browser keeps the clean url
async function channelRewrite(req: NextRequest, channelId: string): Promise<URL | null> {
  const { codes, defaultLocale } = await getChannelLocales(channelId);
  const to = internalPath(req.nextUrl.pathname, channelId, codes, defaultLocale);
  if (!to) return null;
  const url = req.nextUrl.clone();
  url.pathname = to;
  return url;
}

function secretMatches(provided: string | null): boolean {
  const expected = process.env.STOREFRONT_REVALIDATE_SECRET ?? "";
  if (!expected || !provided || provided.length !== expected.length) return false;
  let diff = 0;
  for (let i = 0; i < expected.length; i++) diff |= expected.charCodeAt(i) ^ provided.charCodeAt(i);
  return diff === 0;
}

// The host decides the channel, except for the revalidate route's own warm requests, which name the channel they rebuild
function requestChannel(req: NextRequest): string {
  const warm = req.headers.get(WARM_CHANNEL_HEADER);
  if (isChannelId(warm) && secretMatches(req.headers.get("x-revalidate-secret"))) return warm.toLowerCase();
  return channelForHost(req.headers.get("host") ?? "");
}

export async function proxy(req: NextRequest, event: NextFetchEvent) {
  const pathname = req.nextUrl.pathname;
  if (isChannelTreePath(pathname)) return new NextResponse("Not found", { status: 404 });
  const channelId = requestChannel(req);

  // Route handlers and files only need the channel stamped; one arriving on a host no channel claims passes through bare
  if (!isLegacyMatched(pathname)) {
    const headers = new Headers(req.headers);
    headers.delete(WARM_CHANNEL_HEADER);
    if (channelId) headers.set(CHANNEL_HEADER, channelId);
    else headers.delete(CHANNEL_HEADER);
    return NextResponse.next({ request: { headers } });
  }
  if (!channelId && isPagePath(pathname)) {
    return new NextResponse("No storefront is configured for this host", { status: 404 });
  }

  // Record crawler page fetches (runs for every matched page request).
  reportCrawl(req, event, channelId);

  const url = req.nextUrl;
  const code = url.searchParams.get(PROMO_QUERY_PARAM)?.trim();

  // Proactively rotate a near-expiry customer session before render.
  const session = channelId ? await maybeRefreshSession(req, channelId) : ({ kind: "noop" } as const);

  // The store's consent mode settles the cookie before anything reads it - under opt-out a first visit already counts
  const rawConsent = req.cookies.get(CONSENT_COOKIE)?.value;
  const consentWrite = channelId
    ? resolveConsentCookie(rawConsent, await getConsentMode(channelId), req.headers.get("sec-gpc") === "1")
    : undefined;
  const consent = parseConsent(consentWrite ?? rawConsent);
  const analytics = consent?.analytics ?? false;
  const clickWrites = clickCookieWrites(
    url.searchParams,
    (name) => req.cookies.get(name)?.value,
    consent?.ads ?? false,
    req.headers.get("host") ?? "",
    process.env.NODE_ENV === "production",
  );
  const existingAid = req.cookies.get(AID_COOKIE)?.value;
  const newAid = analytics && !existingAid ? crypto.randomUUID() : undefined;

  // Writes applied to whatever response we return (persist to the browser).
  const applyCookies = (res: NextResponse) => {
    // Re-set even when present: rolls the 1-year expiry per visit, and upgrades a tracker-minted cookie
    // (script-set, so ITP-capped at 7 days on Safari) to a server-set one that actually keeps its expiry
    if (consentWrite !== undefined) {
      res.cookies.set(CONSENT_COOKIE, consentWrite, consentWrite ? CONSENT_COOKIE_OPTS : { ...CONSENT_COOKIE_OPTS, maxAge: 0 });
    }
    if (analytics) res.cookies.set(AID_COOKIE, newAid ?? existingAid!, AID_OPTS);
    else if (existingAid) res.cookies.set(AID_COOKIE, "", { ...AID_OPTS, maxAge: 0 });
    for (const w of clickWrites) res.cookies.set(w.name, w.value, w.options);
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
  if (consentWrite !== undefined) forward[CONSENT_COOKIE] = consentWrite;
  for (const w of clickWrites) if (w.value) forward[w.name] = w.value;
  if (session.kind === "refreshed") forward[ACCESS_TOKEN_COOKIE] = session.accessToken;

  const rewriteTo = channelId ? await channelRewrite(req, channelId) : null;

  const headers = new Headers(req.headers);
  headers.delete(WARM_CHANNEL_HEADER);
  if (channelId) headers.set(CHANNEL_HEADER, channelId);
  else headers.delete(CHANNEL_HEADER);
  if (Object.keys(forward).length > 0) {
    headers.set("cookie", buildForwardedCookie(headers.get("cookie"), forward));
  }
  return applyCookies(
    rewriteTo
      ? NextResponse.rewrite(rewriteTo, { request: { headers } })
      : NextResponse.next({ request: { headers } }),
  );
}

export const config = {
  matcher: [
    "/((?!_next/|.*\\.(?:ico|png|jpe?g|gif|svg|webp|avif|css|js|map|woff2?|ttf|otf)$).*)",
    "/([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})/:path*",
  ],
};
