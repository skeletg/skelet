import { NextRequest, NextResponse } from "next/server";
import { cookies } from "next/headers";

import { ApplyCouponDocument, CreateGuestCartDocument } from "@/app/lib/cart/mutations.gql";
import { CART_COOKIE } from "@/app/lib/cart/read";
import { gqlFetch } from "@/app/lib/graphql";

export async function GET(
  _req: NextRequest,
  ctx: { params: Promise<{ code: string }> },
) {
  const { code } = await ctx.params;
  const trimmed = decodeURIComponent(code ?? "").trim();
  const res = NextResponse.redirect(
    new URL("/cart", _req.nextUrl.origin),
    307,
  );
  if (!trimmed) return res;

  try {
    const cookieStore = await cookies();
    let cartId = cookieStore.get(CART_COOKIE)?.value;
    if (!cartId) {
      const created = await gqlFetch(CreateGuestCartDocument, {});
      cartId = created.data.createGuestCart.id;
      // Must match ensureCartID's cookie exactly: the client reads this cookie
      // (httpOnly:false) and expects the sticky 365-day lifetime
      res.cookies.set(CART_COOKIE, cartId, {
        httpOnly: false,
        sameSite: "lax",
        path: "/",
        secure: process.env.NODE_ENV === "production",
        maxAge: 60 * 60 * 24 * 365,
      });
    }
    await gqlFetch(ApplyCouponDocument, { cartId, code: trimmed });
  } catch {
    // Bad code / backend hiccup - still land on the cart
  }
  return res;
}
