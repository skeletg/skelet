import { redirect, request } from "../request";
import { withLocalePrefix } from "../locale/path";
import { getCurrentChannelId, getCurrentLocale } from "../locale/read";
import { CONFIG_TAG } from "../config/read";
import { GraphQLRequestError, gqlFetch } from "../graphql";
import { SITE_URL, requestHost } from "../site-url";
import { CART_MERGE_FAILED, CART_MERGE_FLAG } from "../cart/merge-flag";
import { SECURE_COOKIES } from "../cookie-policy";
import { CustomerSsoCompleteDocument, CustomerSsoOptionDocument, CustomerSsoStartDocument } from "./operations.gql";
import { sessionCookieOptions } from "./cookies";
import { mergeGuestCartIfNeeded } from "./merge-cart";
import { safeReturnTo, withFlag } from "./return-to";
import { setSessionCookies } from "./session";

// One unprefixed path for every language, since the provider only returns to an address registered with it exactly
export const SSO_PATH = "/account/sso";
// __Host- where cookies are secure, so a sibling subdomain can't plant a sign-in of its own in this browser
export const SSO_COOKIE = SECURE_COOKIES ? "__Host-storefront_sso" : "storefront_sso";
export const SSO_MAX_AGE = 10 * 60;

// locale is the language the shopper started in, since the provider returns everyone to the one unprefixed path
export type Pending = { state: string; binding: string; returnTo: string | null; locale?: string };

// The button's text, null while the store has no provider - a setting like the public config, cached with it
export async function ssoLabel(): Promise<string | null> {
  try {
    const res = await gqlFetch(CustomerSsoOptionDocument, {}, {
      cache: "force-cache",
      revalidate: 300,
      tags: [CONFIG_TAG],
      cacheKeyExtra: [await getCurrentChannelId()],
    });
    return res.data.customerSsoOption?.label ?? null;
  } catch {
    return null;
  }
}

export async function siteOrigin(): Promise<string> {
  const h = await request().headers();
  const host = requestHost(h);
  const proto = (h.get("x-forwarded-proto") ?? "").split(",")[0].trim() || (SITE_URL.startsWith("https:") ? "https" : "http");
  return host ? `${proto}://${host}` : SITE_URL;
}

export async function toLogin(outcome: string, returnTo: string | null = null, locale?: string): Promise<never> {
  const login = withFlag("/account/login", "sso", outcome);
  redirect(await withLocalePrefix(returnTo ? withFlag(login, "returnTo", returnTo) : login, locale ?? (await getCurrentLocale())));
}

// Starts a provider sign-in - the binding stays in an httpOnly cookie, so only this browser can finish. A plain link
// reaches it through a route, so the button works before scripts load and no form-action rule stands in the way
export async function startSso(params: { returnTo: string | null }): Promise<never> {
  const returnTo = safeReturnTo(params.returnTo);
  const locale = await getCurrentLocale();
  let authorizeUrl: string | null = null;
  try {
    const st = (await gqlFetch(CustomerSsoStartDocument, { redirectUri: (await siteOrigin()) + SSO_PATH })).data.customerSsoStart;
    const pending: Pending = { state: st.state, binding: st.binding, returnTo, locale };
    (await request().cookies()).set(SSO_COOKIE, JSON.stringify(pending), { ...sessionCookieOptions(), maxAge: SSO_MAX_AGE });
    authorizeUrl = st.authorizeUrl;
  } catch (err) {
    const code = err instanceof GraphQLRequestError ? err.errors[0]?.extensions?.code : undefined;
    console.error("sso start failed:", code ?? err);
  }
  if (!authorizeUrl || !/^https?:\/\//i.test(authorizeUrl)) return toLogin("failed", returnTo, locale);
  redirect(authorizeUrl);
}

function takePending(raw: string | undefined): Pending | null {
  try {
    const p = raw ? (JSON.parse(raw) as Pending) : null;
    if (!p || typeof p.state !== "string" || typeof p.binding !== "string") return null;
    return { ...p, locale: typeof p.locale === "string" ? p.locale : undefined };
  } catch {
    return null;
  }
}

// Where the provider sends the shopper back: signed in and on to where they were headed, or to the login page with why not
export async function completeSso(params: { code: string | null; state: string | null; error: string | null }): Promise<never> {
  const jar = await request().cookies();
  const pending = takePending(jar.get(SSO_COOKIE)?.value);
  jar.delete(SSO_COOKIE);
  if (!pending || params.state !== pending.state) return toLogin("failed");
  const returnTo = safeReturnTo(pending.returnTo);
  if (params.error || !params.code) return toLogin(params.error === "access_denied" ? "cancelled" : "failed", returnTo, pending.locale);
  let outcome = "failed";
  try {
    const p = (await gqlFetch(CustomerSsoCompleteDocument, { state: pending.state, code: params.code, binding: pending.binding })).data
      .customerSsoComplete;
    if (p.accessToken && p.refreshToken) {
      await setSessionCookies(p.accessToken, p.refreshToken);
      outcome = "ok";
    }
  } catch (err) {
    const code = err instanceof GraphQLRequestError ? err.errors[0]?.extensions?.code : undefined;
    if (code === "SSO_EMAIL_UNVERIFIED") outcome = "unverified";
    else console.error("sso complete failed:", err);
  }
  if (outcome !== "ok") return toLogin(outcome, returnTo, pending.locale);
  const merged = await mergeGuestCartIfNeeded();
  const dest = returnTo ?? "/account";
  redirect(await withLocalePrefix(merged ? dest : withFlag(dest, CART_MERGE_FLAG, CART_MERGE_FAILED), pending.locale ?? (await getCurrentLocale())));
}
