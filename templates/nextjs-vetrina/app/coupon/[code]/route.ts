import { NextRequest, NextResponse } from "next/server";

import { ApplyCouponDocument } from "@skelet/storefront-core/cart/mutations.gql";
import { pendingPromo } from "@skelet/storefront-core/promo";
import { getOwnCartId } from "@skelet/storefront-next/cart/read";
import { gqlFetch } from "@skelet/storefront-next/graphql";

export async function GET(
  _req: NextRequest,
  ctx: { params: Promise<{ code: string }> },
) {
  const { code } = await ctx.params;
  const res = NextResponse.redirect(
    new URL("/cart", _req.nextUrl.origin),
    307,
  );
  let decoded = "";
  try {
    decoded = decodeURIComponent(code ?? "");
  } catch {
    return res;
  }
  const held = pendingPromo(decoded);
  if (!held) return res;

  try {
    const cartId = await getOwnCartId();
    if (cartId) {
      await gqlFetch(ApplyCouponDocument, { cartId, code: held[1] });
    } else {
      // Only an addition creates a guest cart - hold the code for the first one
      res.cookies.set(...held);
    }
  } catch {
    // Bad code / backend hiccup - still land on the cart
  }
  return res;
}
