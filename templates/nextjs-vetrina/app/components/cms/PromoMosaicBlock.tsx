import { PromoMosaic } from "@/app/components/PromoMosaic";
import { listRailProducts } from "@skelet/storefront-next/catalog/read";
import { loadHeader } from "@skelet/storefront-next/header/read";

export async function PromoMosaicBlock() {
  const { categories, collections } = await loadHeader();
  const products = await listRailProducts(categories.map((c) => c.id));
  const deals = products.filter(
    (p) => (p.compareAtPrice ?? 0) > (p.price ?? p.defaultPrice),
  );
  return (
    <PromoMosaic
      deals={deals}
      arrivals={products}
      category={categories[0] ?? null}
      collection={collections[0] ?? null}
    />
  );
}
