import type { ReactNode } from "react";
import { Link } from "@/app/components/LocaleLink";
import { ArrowRightIcon, SparklesIcon, TagIcon } from "lucide-react";

import { browseProducts } from "@skelet/storefront-next/catalog/read";
import type { ProductSummary } from "@skelet/storefront-next/catalog/read";
import type { HeaderCategory, HeaderCollection } from "@skelet/storefront-next/header/read";
import { displayName } from "@skelet/storefront-next/display";
import { categoryHref, collectionHref, productHref } from "@skelet/storefront-next/entity-href";
import { formatPrice } from "@skelet/storefront-next/format";
import { ProductCard } from "@/app/components/ProductCard";
import { RailArrows, RailFrame, RailProvider } from "@/app/components/Rail";
import { CmsSection } from "./Section";

// Composable looks for the grid blocks - layout / card / container registries, each
// extendable without touching a block's data-fetching core. A grid renders
// container(layout(items, renderItem)); layout+container are entity-agnostic, card isn't
//
// SYNC SENTINEL - option ids must match the dashboard catalog
// apps/dashboard/src/features/cms/page-blocks.ts -> BLOCK_REGISTRIES
// (registry names differ there, the ids must not)

export type LayoutView = (props: {
  items: unknown[];
  renderItem: (item: unknown, index: number) => ReactNode;
}) => ReactNode;

export type ProductCardView = (props: { product: ProductSummary }) => ReactNode;
export type CollectionCardView = (props: { collection: HeaderCollection }) => ReactNode;
export type CategoryCardView = (props: { category: HeaderCategory }) => ReactNode;

export type Container = (props: {
  heading?: string;
  href?: string;
  hrefLabel?: string;
  children: ReactNode;
}) => ReactNode;

const layouts = new Map<string, LayoutView>();
const productCards = new Map<string, ProductCardView>();
const collectionCards = new Map<string, CollectionCardView>();
const categoryCards = new Map<string, CategoryCardView>();
const containers = new Map<string, Container>();

export const registerLayout = (id: string, v: LayoutView) => layouts.set(id, v);
export const registerProductCard = (id: string, v: ProductCardView) => productCards.set(id, v);
export const registerCollectionCard = (id: string, v: CollectionCardView) => collectionCards.set(id, v);
export const registerCategoryCard = (id: string, v: CategoryCardView) => categoryCards.set(id, v);
export const registerContainer = (id: string, v: Container) => containers.set(id, v);

export const getLayout = (id?: string): LayoutView => layouts.get(id ?? "") ?? layouts.get("grid")!;
export const getProductCard = (id?: string): ProductCardView => productCards.get(id ?? "") ?? productCards.get("standard")!;
export const getCollectionCard = (id?: string): CollectionCardView => collectionCards.get(id ?? "") ?? collectionCards.get("standard")!;
export const getCategoryCard = (id?: string): CategoryCardView => categoryCards.get(id ?? "") ?? categoryCards.get("standard")!;
export const getContainer = (id?: string): Container => containers.get(id ?? "") ?? containers.get("default")!;

function ProductStandardCard({ product }: { product: ProductSummary }) {
  return <ProductCard product={product} />;
}

function ProductCompactCard({ product }: { product: ProductSummary }) {
  return <ProductCard product={product} variant="compact" />;
}

function ProductOverlayCard({ product }: { product: ProductSummary }) {
  const name = displayName(product.name, product.sku);
  return (
    <Link
      href={productHref(product)}
      className="group relative block aspect-square overflow-hidden rounded-lg border border-border"
    >
      {product.mainImage ? (
        // eslint-disable-next-line @next/next/no-img-element
        <img
          src={product.mainImage.url}
          alt={product.mainImage.altText || name}
          className="absolute inset-0 h-full w-full object-cover transition-transform duration-300 group-hover:scale-105"
        />
      ) : (
        <div className="absolute inset-0 bg-muted" />
      )}
      <div className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/75 via-black/35 to-transparent p-3">
        <div className="line-clamp-2 text-sm font-medium text-white">{name}</div>
        <div className="text-xs text-white/85">{formatPrice(product.defaultPrice, product.defaultCurrencyCode)}</div>
      </div>
    </Link>
  );
}

async function collectionProductCount(id: string): Promise<number> {
  return (await browseProducts({ filter: { collectionId: id }, limit: 1 })).data.totalCount;
}
async function categoryProductCount(id: string): Promise<number> {
  return (await browseProducts({ filter: { categoryId: id }, limit: 1 })).data.totalCount;
}

function ProductCount({ count }: { count: number }) {
  return (
    <span className="block text-xs text-muted-foreground">
      {count} {count === 1 ? "product" : "products"}
    </span>
  );
}

async function CollectionStandardCard({ collection }: { collection: HeaderCollection }) {
  const count = await collectionProductCount(collection.id);
  return (
    <Link
      href={collectionHref(collection)}
      className="glass glass-hover group flex h-full items-center gap-4 rounded-lg p-5"
    >
      <SparklesIcon className="h-5 w-5 shrink-0 text-signal" aria-hidden="true" />
      <span className="min-w-0 flex-1">
        <span className="block text-base font-semibold group-hover:underline">
          {displayName(collection.name, collection.slug)}
        </span>
        <ProductCount count={count} />
      </span>
      <ArrowRightIcon
        className="h-4 w-4 shrink-0 text-muted-foreground transition group-hover:text-foreground"
        aria-hidden="true"
      />
    </Link>
  );
}

async function CollectionCompactCard({ collection }: { collection: HeaderCollection }) {
  const count = await collectionProductCount(collection.id);
  return (
    <Link
      href={collectionHref(collection)}
      className="group flex items-center justify-between gap-3 rounded-xl border border-border p-3 transition hover:bg-accent"
    >
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm font-medium group-hover:underline">
          {displayName(collection.name, collection.slug)}
        </span>
      </span>
      <span className="shrink-0 text-xs text-muted-foreground">
        {count} {count === 1 ? "product" : "products"}
      </span>
    </Link>
  );
}

async function CategoryStandardCard({ category }: { category: HeaderCategory }) {
  const count = await categoryProductCount(category.id);
  return (
    <Link
      href={categoryHref(category)}
      className="glass glass-hover group flex aspect-square flex-col items-center justify-center gap-1 rounded-lg p-4 text-center"
    >
      <TagIcon className="mb-2 h-6 w-6 text-signal" aria-hidden="true" />
      <span className="text-sm font-semibold leading-tight group-hover:underline">
        {displayName(category.name, category.slug)}
      </span>
      <ProductCount count={count} />
    </Link>
  );
}

function CategoryCircleCard({ category }: { category: HeaderCategory }) {
  return (
    <Link
      href={categoryHref(category)}
      className="group flex flex-col items-center gap-3 p-2 text-center"
    >
      <span className="flex h-20 w-20 items-center justify-center rounded-full border border-border bg-muted transition-colors duration-200 group-hover:border-foreground/40 sm:h-24 sm:w-24">
        <TagIcon className="h-8 w-8 text-signal" aria-hidden="true" />
      </span>
      <span className="text-sm font-semibold leading-tight group-hover:underline">
        {displayName(category.name, category.slug)}
      </span>
    </Link>
  );
}

async function CategoryCompactCard({ category }: { category: HeaderCategory }) {
  const count = await categoryProductCount(category.id);
  return (
    <Link
      href={categoryHref(category)}
      className="group flex items-center justify-between gap-3 rounded-xl border border-border p-3 transition hover:bg-accent"
    >
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm font-medium group-hover:underline">
          {displayName(category.name, category.slug)}
        </span>
      </span>
      <span className="shrink-0 text-xs text-muted-foreground">
        {count} {count === 1 ? "product" : "products"}
      </span>
    </Link>
  );
}

function GridLayout({ items, renderItem }: Parameters<LayoutView>[0]) {
  return (
    <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4">
      {items.map((item, i) => (
        <div key={i} className="h-full">
          {renderItem(item, i)}
        </div>
      ))}
    </div>
  );
}

function FeaturedLayout({ items, renderItem }: Parameters<LayoutView>[0]) {
  return (
    <div className="grid auto-rows-fr grid-cols-2 gap-4 lg:grid-cols-4">
      {items.map((item, i) => (
        <div key={i} className={i === 0 ? "col-span-2 row-span-2" : "h-full"}>
          {renderItem(item, i)}
        </div>
      ))}
    </div>
  );
}

function CarouselLayout({ items, renderItem }: Parameters<LayoutView>[0]) {
  return (
    <RailFrame>
      {items.map((item, i) => (
        <div key={i} className="w-52 shrink-0 snap-start sm:w-60">
          {renderItem(item, i)}
        </div>
      ))}
    </RailFrame>
  );
}

function DefaultContainer({ heading, href, hrefLabel, children }: Parameters<Container>[0]) {
  return (
    // The provider wraps the whole section so a carousel inside can drive arrows in its header -
    // for a grid it costs nothing, the arrows render nothing without a track to scroll
    <RailProvider>
      <CmsSection
        heading={heading}
        href={href}
        hrefLabel={hrefLabel}
        actions={<RailArrows />}
      >
        {children}
      </CmsSection>
    </RailProvider>
  );
}

function PlainContainer({ heading, children }: Parameters<Container>[0]) {
  return (
    <section className="mx-auto w-full max-w-8xl px-6 md:px-10 py-8">
      {heading && <h2 className="mb-4 text-xl font-medium tracking-tight text-foreground">{heading}</h2>}
      {children}
    </section>
  );
}

function MutedContainer({ heading, children }: Parameters<Container>[0]) {
  return (
    <section className="border-y border-border bg-muted/40">
      <div className="mx-auto w-full max-w-8xl px-6 md:px-10 py-14">
        {heading && <h2 className="mb-6 text-center text-2xl font-medium tracking-tight text-foreground">{heading}</h2>}
        {children}
      </div>
    </section>
  );
}

registerProductCard("standard", ProductStandardCard);
registerProductCard("compact", ProductCompactCard);
registerProductCard("overlay", ProductOverlayCard);

registerCollectionCard("standard", CollectionStandardCard);
registerCollectionCard("compact", CollectionCompactCard);

registerCategoryCard("standard", CategoryStandardCard);
registerCategoryCard("compact", CategoryCompactCard);
registerCategoryCard("circle", CategoryCircleCard);

registerLayout("grid", GridLayout);
registerLayout("featured", FeaturedLayout);
registerLayout("carousel", CarouselLayout);

registerContainer("default", DefaultContainer);
registerContainer("plain", PlainContainer);
registerContainer("muted", MutedContainer);
