import type { AstroCookies } from "astro";

import { CART_COOKIE } from "@skelet/storefront-core/cart/read";

// The cart itself is the core's - its cookie, its guest cart, a signed-in customer's own. These are the shop's: the
// badge, its pop and a message carried across a redirect
// Readable by the page's one inline script, so cached HTML stays the same for everyone and the badge fills in client-side
const COUNT = "cart_count";
// Read by the motion script on the next page, which clears it
const BUMP = "cart_bump";
const FLASH = "flash";
const MONTH = 60 * 60 * 24 * 30;

const opts = (httpOnly: boolean) => ({ path: "/", sameSite: "lax" as const, secure: import.meta.env.PROD, httpOnly, maxAge: MONTH });

export function rememberCart(cookies: AstroCookies, cart: { lines: { quantity: number }[] }): void {
  cookies.set(COUNT, String(cart.lines.reduce((n, l) => n + l.quantity, 0)), opts(false));
}

// Tells the next page's motion script to pop the cart badge once
export function bumpBadge(cookies: AstroCookies): void {
  cookies.set(BUMP, "1", { ...opts(false), maxAge: 60 });
}

// After an order, or for a cart the backend no longer has - the core's cookie goes with the badge
export function forgetCart(cookies: AstroCookies): void {
  cookies.delete(CART_COOKIE, { path: "/" });
  cookies.delete(COUNT, { path: "/" });
}

export type CartAction =
  | { intent: "add"; sku: string; quantity: number }
  | { intent: "update"; lineId: string; quantity: number }
  | { intent: "remove"; lineId: string };

// What a cart form posted, or null when it makes no sense. Remove posts no quantity at all, so an emptied
// box is a typo rather than a remove - while an update to 0 is one
export function cartAction(form: FormData): CartAction | null {
  const raw = form.get("quantity");
  const quantity = raw === null ? 1 : raw === "" ? NaN : Number(raw);
  if (!Number.isFinite(quantity) || quantity < 0) return null;
  const intent = form.get("intent");
  const lineId = String(form.get("lineId") ?? "");
  if (intent === "add") return { intent, sku: String(form.get("sku") ?? ""), quantity };
  if (intent === "remove" || (intent === "update" && quantity === 0)) return { intent: "remove", lineId };
  if (intent === "update") return { intent, lineId, quantity };
  return null;
}

// One message carried across a redirect, then gone
export function setFlash(cookies: AstroCookies, message: string): void {
  cookies.set(FLASH, message, { ...opts(true), maxAge: 60 });
}

export function takeFlash(cookies: AstroCookies): string | undefined {
  const msg = cookies.get(FLASH)?.value;
  if (msg) cookies.delete(FLASH, { path: "/" });
  return msg;
}
