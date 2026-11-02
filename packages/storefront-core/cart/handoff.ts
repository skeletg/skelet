import { request, rethrowControlFlow } from "../request";
import { getAccessToken } from "../auth/session";
import { cookieOptions } from "../cookie-policy";
import { gqlFetch, GraphQLRequestError } from "../graphql";
import { CART_COOKIE } from "./read";
import { RedeemCartHandoffDocument } from "./handoff.gql";

export type HandoffState = "ok" | "expired" | "invalid" | "failed";

// Where a shopper's AI agent sends them to check out: the agent's cart is copied into the shopper's own, and a guest's
// cookie then points at the cart it landed in. Opening the link again copies nothing more
export async function redeemHandoff(rawToken: string): Promise<HandoffState> {
  const token = (rawToken ?? "").trim();
  if (!token) return "invalid";
  const signedIn = Boolean(await getAccessToken());
  const jar = await request().cookies();
  try {
    const res = await gqlFetch(RedeemCartHandoffDocument, { token, cartId: signedIn ? null : (jar.get(CART_COOKIE)?.value ?? null) });
    if (!signedIn) jar.set(CART_COOKIE, res.data.redeemCartHandoff.id, cookieOptions(60 * 60 * 24 * 365));
    return "ok";
  } catch (err) {
    rethrowControlFlow(err);
    const code = err instanceof GraphQLRequestError ? err.errors[0]?.extensions?.code : undefined;
    if (code === "CART_HANDOFF_EXPIRED") return "expired";
    return code === "CART_HANDOFF_INVALID" ? "invalid" : "failed";
  }
}
