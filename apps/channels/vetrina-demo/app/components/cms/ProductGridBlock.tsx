import type { Block } from "./blocks";
import { readNumber, readString } from "./blocks";
import { getContainer, getLayout, getProductCard } from "./variants";
import { browseProducts } from "@/app/lib/catalog/read";
import type { ProductSummary } from "@/app/lib/catalog/read";

// product_grid - newest / collection / category, all through the deduped browseProducts
// loader (OpenSearch-backed, degrades to empty on outage so the page still renders)
export async function ProductGridBlock({ block }: { block: Block }) {
  const heading = readString(block.props, "heading");
  const source = readString(block.props, "source") ?? "newest";
  const limit = readNumber(block.props, "limit") ?? 12;
  const collectionId = readString(block.props, "collectionId");
  const categoryId = readString(block.props, "categoryId");
  const onlySale = readString(block.props, "onlySale") === "sale";

  const filter =
    source === "collection" && collectionId
      ? { collectionId }
      : source === "category" && categoryId
        ? { categoryId }
        : undefined;

  const res = await browseProducts({ filter, limit });
  let products = res.data.items as unknown as ProductSummary[];
  if (onlySale) {
    products = products.filter(
      (p) => (p.compareAtPrice ?? 0) > (p.price ?? p.defaultPrice),
    );
  }

  if (products.length === 0) return null;

  // Presentation is composed from three registries (props.variant / card / container) - the core block only fetches
  // data, so new looks are added by registering components, never by editing this file
  const Card = getProductCard(readString(block.props, "card"));
  const renderLayout = getLayout(readString(block.props, "variant"));
  const renderContainer = getContainer(readString(block.props, "container"));

  return renderContainer({
    heading,
    href: "/products",
    hrefLabel: "View all",
    children: renderLayout({
      items: products,
      renderItem: (p) => Card({ product: p as ProductSummary }),
    }),
  });
}
