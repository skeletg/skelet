import { redirect, request } from "../request";
import { gqlFetch } from "../graphql";
import { CustomerSsoStartDocument } from "./operations.gql";
import { sessionCookieOptions } from "./cookies";
import { safeReturnTo } from "./return-to";
import { SSO_COOKIE, SSO_MAX_AGE, SSO_PATH, siteOrigin, toLogin, type Pending } from "./sso";

// Sends the shopper to the provider - the binding stays in an httpOnly cookie, so only this browser can finish
export async function ssoStartAction(formData: FormData): Promise<void> {
  const returnTo = safeReturnTo(formData.get("returnTo"));
  let authorizeUrl: string | null = null;
  try {
    const st = (await gqlFetch(CustomerSsoStartDocument, { redirectUri: (await siteOrigin()) + SSO_PATH })).data.customerSsoStart;
    const pending: Pending = { state: st.state, binding: st.binding, returnTo };
    (await request().cookies()).set(SSO_COOKIE, JSON.stringify(pending), { ...sessionCookieOptions(), maxAge: SSO_MAX_AGE });
    authorizeUrl = st.authorizeUrl;
  } catch (err) {
    console.error("sso start failed:", err);
  }
  if (!authorizeUrl) await toLogin("failed");
  redirect(authorizeUrl!);
}
