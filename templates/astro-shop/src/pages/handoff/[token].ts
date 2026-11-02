import type { APIRoute } from "astro";

import { redeemHandoff, type HandoffState } from "@skelet/storefront-core/cart/handoff";

import { setFlash } from "../../lib/cart";

const COPY: Record<Exclude<HandoffState, "ok">, string> = {
  expired: "That checkout link has expired - ask your assistant for a new one.",
  invalid: "That checkout link isn't valid.",
  failed: "We couldn't open that checkout link just now - please try it again in a moment.",
};

// Where a shopper's AI agent sends them to check out - its cart joins theirs, and they land on it to look it over
export const GET: APIRoute = async ({ params, cookies, redirect }) => {
  const state = await redeemHandoff(params.token ?? "");
  if (state !== "ok") setFlash(cookies, COPY[state]);
  return redirect("/cart", 303);
};
