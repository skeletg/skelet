import { cookieOptions } from "./cookie-policy";

export const PROMO_PENDING_COOKIE = "storefront_pending_promo";

const HELD_FOR = 60 * 60 * 24 * 7;
const CODE = /^[\x21-\x7e]{1,64}$/;

export function pendingPromo(raw: string | null | undefined): [string, string, ReturnType<typeof cookieOptions>] | null {
  const code = (raw ?? "").trim().toUpperCase();
  return CODE.test(code) ? [PROMO_PENDING_COOKIE, code, cookieOptions(HELD_FOR)] : null;
}
