import { redirect, request } from "../request";
import { withLocalePrefix } from "../locale/path";
import { getCurrentLocale } from "../locale/read";
import { GraphQLRequestError, gqlFetch } from "../graphql";
import { SITE_URL } from "../site-url";
import { CART_MERGE_FAILED, CART_MERGE_FLAG } from "../cart/merge-flag";
import { CustomerSsoCompleteDocument, CustomerSsoOptionDocument } from "./operations.gql";
import { mergeGuestCartIfNeeded } from "./merge-cart";
import { setSessionCookies } from "./session";

// One unprefixed path for every language, since the provider only returns to an address registered with it exactly
export const SSO_PATH = "/account/sso";
export const SSO_COOKIE = "storefront_sso";
export const SSO_MAX_AGE = 10 * 60;

export type Pending = { state: string; binding: string; returnTo: string | null };

// The button's text, null while the store has no provider
export async function ssoLabel(): Promise<string | null> {
  try {
    return (await gqlFetch(CustomerSsoOptionDocument, {})).data.customerSsoOption?.label ?? null;
  } catch {
    return null;
  }
}

export async function siteOrigin(): Promise<string> {
  const h = await request().headers();
  const host = (h.get("x-forwarded-host") ?? h.get("host") ?? "").split(",")[0].trim();
  const proto = (h.get("x-forwarded-proto") ?? "").split(",")[0].trim() || (SITE_URL.startsWith("https:") ? "https" : "http");
  return host ? `${proto}://${host}` : SITE_URL;
}

export async function toLogin(outcome: string): Promise<never> {
  redirect(await withLocalePrefix(`/account/login?sso=${outcome}`, await getCurrentLocale()));
}

function takePending(raw: string | undefined): Pending | null {
  try {
    const p = raw ? (JSON.parse(raw) as Pending) : null;
    return p && typeof p.state === "string" && typeof p.binding === "string" ? p : null;
  } catch {
    return null;
  }
}

// Where the provider sends the shopper back: signed in and on to where they were headed, or to the login page with why not
export async function completeSso(params: { code: string | null; state: string | null; error: string | null }): Promise<never> {
  const jar = await request().cookies();
  const pending = takePending(jar.get(SSO_COOKIE)?.value);
  jar.delete(SSO_COOKIE);
  if (params.error) return toLogin("cancelled");
  if (!pending || !params.code || params.state !== pending.state) return toLogin("failed");
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
  if (outcome !== "ok") return toLogin(outcome);
  const merged = await mergeGuestCartIfNeeded();
  let dest = pending.returnTo ?? "/account";
  if (!merged) dest += `${dest.includes("?") ? "&" : "?"}${CART_MERGE_FLAG}=${CART_MERGE_FAILED}`;
  redirect(await withLocalePrefix(dest, await getCurrentLocale()));
}
