// Thin route handler around the front GraphQL `shippingQuotes` query

import { cookies } from "next/headers";
import { NextResponse } from "next/server";

import { CART_COOKIE } from "@/app/lib/cart/read";

import { ShippingQuotesDocument } from "@/app/lib/checkout/operations.gql";
import { gqlFetch } from "@/app/lib/graphql";

export async function GET(req: Request) {
  const url = new URL(req.url);
  const country = (url.searchParams.get("country") ?? "")
    .trim()
    .toUpperCase();
  if (!country || country.length !== 2) {
    return NextResponse.json({ quotes: [] });
  }
  try {
    const region = (url.searchParams.get("region") ?? "").trim();
    const postal = (url.searchParams.get("postal") ?? "").trim();
    const res = await gqlFetch(ShippingQuotesDocument, {
      countryCode: country,
      cartId: (await cookies()).get(CART_COOKIE)?.value ?? null,
      regionCode: region || null,
      postalCode: postal || null,
    });
    return NextResponse.json({ quotes: res.data.shippingQuotes });
  } catch {
    // Unreachable backend / unknown country → empty list. The UI
    // already handles the "no methods available" branch
    return NextResponse.json({ quotes: [] });
  }
}
