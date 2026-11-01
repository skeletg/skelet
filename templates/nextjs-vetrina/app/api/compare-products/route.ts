import { NextRequest, NextResponse } from "next/server";

import { CompareProductsDocument } from "@skelet/storefront-core/catalog/operations.gql";
import { gqlFetch } from "@skelet/storefront-next/graphql";
import { getChannel } from "@skelet/storefront-next/channel/read";

export async function GET(req: NextRequest) {
  const ids = (req.nextUrl.searchParams.get("ids") ?? "")
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean)
    .slice(0, 4);
  if (ids.length === 0) return NextResponse.json({ products: [] });
  try {
    const [res, channel] = await Promise.all([
      gqlFetch(CompareProductsDocument, { ids }),
      getChannel(),
    ]);
    return NextResponse.json({
      products: res.data.productsByIds,
      currency: channel?.currencyCode ?? "USD",
    });
  } catch {
    return NextResponse.json({ products: [] });
  }
}
