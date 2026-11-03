import "../setup";

import { redirect } from "next/navigation";

import { redeemHandoff } from "@skelet/storefront-core/cart/handoff";

export { redeemHandoff, type HandoffState } from "@skelet/storefront-core/cart/handoff";

// The /handoff/<token> route: the shopper lands on their cart to look over what the agent picked, with ?handoff=<state> when the link didn't open
export async function GET(_req: Request, ctx: { params: Promise<{ token: string }> }): Promise<never> {
  const state = await redeemHandoff((await ctx.params).token);
  redirect(state === "ok" ? "/cart" : `/cart?handoff=${state}`);
}
