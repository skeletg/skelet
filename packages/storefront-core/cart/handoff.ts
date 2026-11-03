import { request, rethrowControlFlow } from "../request";
import { gqlFetch, GraphQLRequestError } from "../graphql";
import { setCartCookie } from "./actions";
import { CART_COOKIE } from "./read";
import { RedeemCartHandoffDocument } from "./handoff.gql";

export type HandoffState = "ok" | "expired" | "invalid" | "failed";

// The backend says whether the lines landed in a guest cart, so the cookie follows them even past a lapsed session
export async function redeemHandoff(rawToken: string): Promise<HandoffState> {
  const token = (rawToken ?? "").trim();
  if (!token) return "invalid";
  const jar = await request().cookies();
  try {
    const { guestCart, cart } = (await gqlFetch(RedeemCartHandoffDocument, { token, cartId: jar.get(CART_COOKIE)?.value ?? null })).data.redeemCartHandoff;
    if (guestCart) setCartCookie(jar, cart.id);
    return "ok";
  } catch (err) {
    rethrowControlFlow(err);
    const code = err instanceof GraphQLRequestError ? err.errors[0]?.extensions?.code : undefined;
    if (code === "CART_HANDOFF_EXPIRED") return "expired";
    return code === "CART_HANDOFF_INVALID" ? "invalid" : "failed";
  }
}
