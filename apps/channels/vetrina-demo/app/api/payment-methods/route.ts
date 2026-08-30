// Thin route handler around the front GraphQL `paymentMethods` query

import { NextResponse } from "next/server";

import { PaymentMethodsDocument } from "@/app/lib/checkout/operations.gql";
import { gqlFetch } from "@/app/lib/graphql";

export async function GET(req: Request) {
  const url = new URL(req.url);
  const country = (url.searchParams.get("country") ?? "")
    .trim()
    .toUpperCase();
  if (!country || country.length !== 2) {
    return NextResponse.json({ methods: [] });
  }
  try {
    const res = await gqlFetch(PaymentMethodsDocument, {
      countryCode: country,
    });
    return NextResponse.json({ methods: res.data.paymentMethods });
  } catch {
    // Unreachable backend / unknown country → empty list. The UI
    // already handles the "no methods available" branch
    return NextResponse.json({ methods: [] });
  }
}
