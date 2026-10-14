import { NextRequest, NextResponse } from "next/server";
import { getCurrentChannelLocales } from "@/app/lib/locale/read";
import { storeRequestSchema } from "@/app/lib/stores/request";
import { readPublicStores, readStoreProduct } from "@/app/lib/stores/read";
import { gqlFetch, errorClass } from "@/app/lib/graphql";
import { SearchProductsDocument } from "@/app/lib/search/operations.gql";

const response = (data: unknown, status = 200) => NextResponse.json(data, { status, headers: { "Cache-Control": "no-store" } });
export async function GET(req: NextRequest) {
  const parsed = storeRequestSchema.safeParse(Object.fromEntries(req.nextUrl.searchParams));
  if (!parsed.success) return response({ error: "Invalid request" }, 400);
  const { action, inStockOnly, pickupOnly, locale: requestedLocale, ...input } = parsed.data;
  try {
    const locales = await getCurrentChannelLocales();
    const locale = requestedLocale && locales.codes.includes(requestedLocale) ? requestedLocale : locales.defaultLocale;
    if (action !== "stores") {
      const gate = await readPublicStores({ limit: 1 });
      if (!gate.enabled || !gate.stockCheckingAvailable) return response({ error: "Unavailable" }, 404);
      if (action === "product") {
        if (!input.productId) return response({ error: "Invalid request" }, 400);
        return response({ product: await readStoreProduct(input.productId, locale) });
      }
      if (!input.search?.trim()) return response({ items: [] });
      const result = await gqlFetch(SearchProductsDocument, { query: input.search, limit: 12 }, { locale });
      return response({ items: result.data.searchProducts.items });
    }
    return response(await readPublicStores({ ...input, inStockOnly: inStockOnly === "true", pickupOnly: pickupOnly === "true", limit: 20 }, locale));
  } catch (err) {
    console.error("Store availability request failed", err);
    return response({ error: "Unable to load stores" }, errorClass(err) === "NOT_FOUND" ? 404 : 503);
  }
}
