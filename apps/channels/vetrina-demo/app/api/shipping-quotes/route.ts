// Thin route handler around the front GraphQL `shippingQuotes` query

import { NextResponse } from "next/server";

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
    const res = await gqlFetch(ShippingQuotesDocument, {
      countryCode: country,
    });
    return NextResponse.json({ quotes: res.data.shippingQuotes });
  } catch {
    // Unreachable backend / unknown country → empty list. The UI
    // already handles the "no methods available" branch
    return NextResponse.json({ quotes: [] });
  }
}
