import { NextRequest, NextResponse } from "next/server";

import { ProductsByIdsDocument } from "@/app/lib/catalog/operations.gql";
import { gqlFetch } from "@/app/lib/graphql";

export async function GET(req: NextRequest) {
  const ids = (req.nextUrl.searchParams.get("ids") ?? "")
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean)
    .slice(0, 12);
  if (ids.length === 0) return NextResponse.json({ products: [] });
  try {
    const res = await gqlFetch(ProductsByIdsDocument, { ids });
    return NextResponse.json({ products: res.data.productsByIds });
  } catch {
    return NextResponse.json({ products: [] });
  }
}
