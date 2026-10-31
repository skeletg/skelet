import { NextRequest, NextResponse } from "next/server";

import { ApplyCouponDocument } from "@skelet/storefront-next/cart/mutations.gql";
import { getOwnCartId, PROMO_PENDING_COOKIE } from "@skelet/storefront-next/cart/read";
import { gqlFetch } from "@skelet/storefront-next/graphql";

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
    const cartId = await getOwnCartId();
    if (cartId) {
      await gqlFetch(ApplyCouponDocument, { cartId, code: trimmed });
    } else {
      // Only an addition creates a guest cart - hold the code for the first one
      res.cookies.set(PROMO_PENDING_COOKIE, trimmed, {
        httpOnly: true,
        sameSite: "lax",
        path: "/",
        secure: process.env.NODE_ENV === "production",
        maxAge: 60 * 60 * 24 * 30,
      });
    }
  } catch {
    // Bad code / backend hiccup - still land on the cart
  }
  return res;
}
