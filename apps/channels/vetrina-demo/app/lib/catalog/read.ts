// Storefront catalog queries + the trimmed TS shapes for the fields we actually render

import type { ProductOptionRule } from "@/app/lib/optionRules";
import { cache } from "react";

import {
  BrowseProductsDocument,
  BrowseProductsSlimDocument,
  type BrowseProductsSlimQuery,
  ProductRecommendationsDocument,
  type BrowseProductsQuery,
  type BrowseProductsQueryVariables,
  CategoryDocument,
  CollectionDocument,
  CollectionsDocument,
  ProductDocument,
  ProductConfiguratorDocument,
  type ProductConfiguratorPicturesFragment,
  ProductSortOptionsDocument,
  type ProductSortOptionsQuery,
  SynapseAttributesDocument,
  CartRecommendationsDocument,
} from "@/app/lib/catalog/operations.gql";
import { gqlFetch } from "@/app/lib/graphql";
import { normalizeBrowseVars } from "@/app/lib/catalog/browse-vars";
import { surfaceCacheOpts } from "@/app/lib/config/read";

// Both match a backend revalidate.Surface (internal/catalog/cache.go), whose cache/<key>/{enabled,ttl_seconds} Settings → Cache drives through surfaceCacheOpts
export const PRODUCT_SURFACE = "product";
// Bust tags: category:<id>, collection:<id>, catalog:browse, catalog:all
export const CATALOG_SURFACE = "catalog";

export type ProductMedia = {
  id: string;
  url: string;
  altText: string;
  mimeType: string;
  width: number;
  height: number;
};

export type AttributeValue = {
  id: string;
  name: string;
  value: string;
};

export type ProductAttributeReference = {
  targetType: string;
  targetId: string;
  label: string;
  url: string;
};

export type ProductAttributeValue = {
  attribute: {
    id: string;
    slug: string;
    name: string;
    unit: string;
    synapseTrack: boolean;
  };
  selectedValues: AttributeValue[];
  scalarValue: unknown;
  references: ProductAttributeReference[];
};

// PromotionDisplayMedia is the slim subset of fields we actually render on PLP / PDP. Backend returns full ProductMedia but the storefront
// fragment intentionally picks only what's needed for the overlay / banner - narrower type = smaller bundle + clearer intent at call sites
export type PromotionDisplayMedia = {
  id: string;
  url: string;
  altText: string;
  width: number;
  height: number;
};

export type PromotionalDisplay = {
  ruleId: string;
  ruleSlug: string;
  publicLabel: string;
  labelMedia: PromotionDisplayMedia | null;
  bannerMedia: PromotionDisplayMedia | null;
  pillText: string;
  pillTextColor: string;
  pillBgColor: string;
};

export type ProductSummary = {
  id: string;
  url: string;
  sku: string;
  name: string | null;
  inStock: boolean;
  kind: "SIMPLE" | "CONFIGURABLE" | "BUNDLE" | "GROUPED";
  defaultPrice: number;
  defaultCurrencyCode: string;
  // Effective sell price (channel listing + active specials)
  price?: number | null;
  compareAtPrice?: number | null;
  onSale?: boolean | null;
  minOrderQty?: number | null;
  maxOrderQty?: number | null;
  quantityStep?: number | null;
  quantityDecimals?: number | null;
  mainImage: ProductMedia | null;
  thumbnail: ProductMedia | null;
  promotionalDisplays: PromotionalDisplay[];
  reviewAggregates?: { count: number; overallAverage?: number | null } | null;
};

export type ProductVariant = {
  id: string;
  // The variant's own canonical URL - drives the address-bar swap when the shopper
  // picks this variant on a configurable PDP
  url: string;
  sku: string;
  name: string | null;
  inStock: boolean;
  salableQuantity: number | null;
  defaultPrice: number;
  // Channel-effective sell price (listing override + active special) - display this,
  // defaultPrice is only the base fallback
  price: number;
  compareAtPrice: number | null;
  onSale: boolean;
  defaultCurrencyCode: string;
  mainImage: ProductMedia | null;
  media: ProductMedia[];
  attributeValues: ProductAttributeValue[];
  shortDescription: unknown;
  description: unknown;
  metaTitle: string | null;
  options: ProductCustomOption[];
  optionRules: ProductOptionRule[];
};

export type ConfigurableOption = {
  id: string;
  slug: string;
  name: string;
};

export type BundleInputType =
  | "RADIO"
  | "DROPDOWN"
  | "CHECKBOX"
  | "MULTISELECT";

export type BundleSelection = {
  id: string;
  quantity: number;
  isDefault: boolean;
  sortOrder: number;
  product: {
    id: string;
    url: string;
    sku: string;
    name: string | null;
    inStock: boolean;
    defaultPrice: number;
    price: number;
    defaultCurrencyCode: string;
    mainImage: ProductMedia | null;
  };
};

export type BundleOption = {
  id: string;
  title: string;
  inputType: BundleInputType;
  isRequired: boolean;
  sortOrder: number;
  // Pick-count bounds for checkbox/multiselect - 0 is no explicit minimum, null no maximum
  minSelections: number;
  maxSelections?: number | null;
  selections: BundleSelection[];
};

export type GroupedMember = {
  id: string;
  defaultQuantity: number;
  sortOrder: number;
  member: {
    id: string;
    url: string;
    sku: string;
    name: string | null;
    inStock: boolean;
    defaultPrice: number;
    price: number;
    defaultCurrencyCode: string;
    salableQuantity: number | null;
    minOrderQty?: number | null;
    maxOrderQty?: number | null;
    quantityStep?: number | null;
    quantityDecimals?: number | null;
    mainImage: ProductMedia | null;
    options: ProductCustomOption[];
    optionRules: ProductOptionRule[];
  };
};

export type ProductCustomOptionImage = { url: string; altText: string; width: number | null; height: number | null };

export type ProductCustomOptionValue = {
  id: string;
  title: string;
  // Filled from the configurator query - shown as a tile to pick rather than a plain label
  image?: ProductCustomOptionImage | null;
  priceValue: number;
  priceType: string;
  quantity: number;
  isDefault: boolean;
  forcedLineQty: number | null;
  sourceProduct: {
    id: string;
    url: string;
    sku: string;
    name: string | null;
    defaultPrice: number;
    defaultCurrencyCode: string;
  } | null;
};

export type ProductCustomOption = {
  id: string;
  inputType: string; // field|area|dropdown|radio|checkbox|multiselect|date|date_time|time|file
  isRequired: boolean;
  sortOrder: number;
  title: string;
  helpText: string;
  maxChars: number | null;
  maxFileBytes: number | null;
  fileExtensions: string | null;
  priceValue: number | null;
  priceType: string;
// Gates this option on earlier picks (null = always visible) { logic, rules: [{ option_id, op, value_ids }], groups } -
// the server re-evaluates on add-to-cart, optionVisibility.ts mirrors it for live show/hide
  visibleIf: unknown;
  values: ProductCustomOptionValue[];
  // Filled from the configurator query - shown beside the option
  image?: ProductCustomOptionImage | null;
};

export type ProductDetail = ProductSummary & {
  backorderable: boolean;
  // Quantity-break prices for the current caller (channel + group scoped)
  tierPrices: { minQty: number; price: number }[];
  shortDescription: unknown;
  description: unknown;
  metaTitle: string | null;
  metaDescription: string | null;
  salableQuantity: number | null;
  media: ProductMedia[];
  // Explicit OG role only - null means the PDP falls back to the first gallery image
  ogImage: ProductMedia | null;
  attributeValues: ProductAttributeValue[];
  variantMode: "ATTRIBUTE" | "LIST" | null;
  configurableOptions: ConfigurableOption[];
  children: ProductVariant[];
  partOf: {
    configurable: { parent: { id: string } } | null;
  };
  bundleOptions: BundleOption[];
  groupedMembers: GroupedMember[];
  options: ProductCustomOption[];
  optionRules: ProductOptionRule[];
  // Every option at once, or one step at a time - from the configurator query, LIST without options
  configuratorLayout: "LIST" | "STEPS";
  // Recurring terms for SUBSCRIPTION products (null otherwise); drives the PDP's "$X /
  // month" display
  subscriptionTerms: {
    intervalUnit: string;
    intervalCount: number;
    trialDays: number;
  } | null;
  categories: { id: string; slug: string; name: string | null }[];
  primaryCategory:
    | {
        id: string;
        slug: string;
        name: string | null;
        urlPath: string;
        ancestors: {
          id: string;
          slug: string;
          name: string | null;
          urlPath: string;
        }[];
      }
    | null;
  collections: { id: string; slug: string; name: string | null }[];
};

// All reads in this file are React.cache-wrapped so a render tree with multiple sibling server components hitting the
// same id (or the same listing) shares a single backend roundtrip per request
export async function listRailProducts(
  categoryIds: string[],
): Promise<ProductSummary[]> {
  const counts = await Promise.all(
    categoryIds.map(async (id) => ({
      id,
      count: (await browseProducts({ filter: { categoryId: id }, limit: 1 }))
        .data.totalCount,
    })),
  );
  const best = counts.reduce(
    (a, b) => (b.count > a.count ? b : a),
    { id: "", count: 0 },
  );
  if (!best.id || best.count === 0) return (await listProducts()).products;
  const res = await browseProducts({ filter: { categoryId: best.id }, limit: 12 });
  return res.data.items as unknown as ProductSummary[];
}

export const listProducts = cache(async (): Promise<{
  products: ProductSummary[];
}> => {
  const res = await browseProducts({ limit: 12 });
  // The generated query type and the hand-written ProductSummary are parallel definitions that match field-for-field at runtime but drift
  // in nested shapes (JSON scalars typed `unknown`, inline vs named media/promo types), which TS can't reconcile structurally
  return { products: res.data.items as unknown as ProductSummary[] };
});

export type CategorySummary = {
  id: string;
  slug: string;
  name: string | null;
  urlPath: string;
  shortDescription: unknown;
  description: unknown;
  ancestors: { id: string; slug: string; urlPath: string; name: string | null }[];
  children: { id: string; slug: string; urlPath: string; name: string | null }[];
};

export type CollectionDetail = {
  id: string;
  slug: string;
  name: string | null;
  description: unknown;
  metaTitle: string | null;
  metaDescription: string | null;
  coverImage: {
    url: string;
    altText: string;
    width: number;
    height: number;
  } | null;
};

export type CollectionListItem = {
  id: string;
  slug: string;
  name: string | null;
  description: unknown;
  coverImage: {
    url: string;
    altText: string;
  } | null;
};

export const listCollections = cache(async (): Promise<{
  collections: CollectionListItem[];
}> => {
  const cacheOpts = await surfaceCacheOpts(CATALOG_SURFACE, {
    tags: ["catalog:collections", "catalog:all"],
  });
  const res = await gqlFetch(CollectionsDocument, {}, cacheOpts);
  return {
    collections: res.data.collections,
  };
});

export const getCollection = cache(async (id: string): Promise<{
  collection: CollectionDetail | null;
}> => {
  try {
    const cacheOpts = await surfaceCacheOpts(CATALOG_SURFACE, {
      tags: [`collection:${id}`, "catalog:all"],
    });
    const res = await gqlFetch(CollectionDocument, { id }, cacheOpts);
    return { collection: res.data.collection as unknown as CollectionDetail | null };
  } catch {
    return { collection: null };
  }
});

export const getCategory = cache(async (id: string): Promise<{
  category: CategorySummary | null;
}> => {
  try {
    // Category pages are identical for every visitor → cached for all when the catalog surface is enabled. Busts on
    // category:<id> + catalog:all; dev = always fresh (see surfaceCacheOpts)
    const cacheOpts = await surfaceCacheOpts(CATALOG_SURFACE, {
      tags: [`category:${id}`, "catalog:all"],
    });
    const res = await gqlFetch(CategoryDocument, { id }, cacheOpts);
    return { category: res.data.category as unknown as CategorySummary | null };
  } catch (err) {
    if (
      err instanceof Error &&
      /not found|no rows|invalid/i.test(err.message)
    ) {
      return { category: null };
    }
    throw err;
  }
});

export const getProduct = cache(async (id: string): Promise<{
  product: ProductDetail | null;
}> => {
  try {
// Cached for everyone, tagged product:<id> so a price/stock/delete busts it via /api/revalidate. Per-id only - a
// catalog-wide tag would stampede the renderer
    const cacheOpts = await surfaceCacheOpts(PRODUCT_SURFACE, {
      tags: [`product:${id}`],
    });
    const res = await gqlFetch(ProductDocument, { id }, cacheOpts);
    const product = res.data.product as unknown as ProductDetail | null;
    return { product: product ? await withConfigurator(product, cacheOpts) : null };
  } catch (err) {
    // The backend returns a GraphQL error (not a null product) when the ID doesn't
    // exist or isn't visible to the storefront
    if (
      err instanceof Error &&
      /not found|no rows|invalid/i.test(err.message)
    ) {
      return { product: null };
    }
    throw err;
  }
});

// The Product query sits at the front surface's complexity cap, so option rules and pictures come in a query of their
// own - and only when the product, a variant or a grouped member has options for them to belong to
async function withConfigurator(
  product: ProductDetail,
  cacheOpts: Parameters<typeof gqlFetch>[2],
): Promise<ProductDetail> {
  const empty = { ...product, optionRules: [] as ProductOptionRule[], configuratorLayout: "LIST" as const };
  const hasOptions =
    product.options.length > 0 ||
    product.children.some((c) => c.options.length > 0) ||
    product.groupedMembers.some((m) => m.member.options.length > 0);
  if (!hasOptions) return empty;
  const res = await gqlFetch(ProductConfiguratorDocument, { id: product.id }, cacheOpts);
  const extra = res.data.product;
  if (!extra) return empty;
  const byChild = new Map(extra.children.map((c) => [c.id, c]));
  const byMember = new Map(extra.groupedMembers.map((m) => [m.member.id, m.member]));
  return {
    ...product,
    configuratorLayout: extra.configuratorLayout,
    options: withPictures(product.options, extra.options),
    optionRules: extra.optionRules as ProductOptionRule[],
    children: product.children.map((c) => ({
      ...c,
      options: withPictures(c.options, byChild.get(c.id)?.options ?? []),
      optionRules: (byChild.get(c.id)?.optionRules ?? []) as ProductOptionRule[],
    })),
    groupedMembers: product.groupedMembers.map((m) => ({
      ...m,
      member: {
        ...m.member,
        options: withPictures(m.member.options, byMember.get(m.member.id)?.options ?? []),
        optionRules: (byMember.get(m.member.id)?.optionRules ?? []) as ProductOptionRule[],
      },
    })),
  };
}

function withPictures(options: ProductCustomOption[], pictures: readonly ProductConfiguratorPicturesFragment[]): ProductCustomOption[] {
  const byId = new Map(pictures.map((p) => [p.id, p]));
  return options.map((o) => {
    const p = byId.get(o.id);
    if (!p) return o;
    const values = new Map(p.values.map((v) => [v.id, v.image ?? null]));
    return { ...o, image: p.image ?? null, values: o.values.map((v) => ({ ...v, image: values.get(v.id) ?? null })) };
  });
}

export type BrowseProductsResult = BrowseProductsQuery["browseProducts"];
export type BrowseProductsItem = BrowseProductsResult["items"][number];
export type BrowseFacetGroup = BrowseProductsResult["facets"][number];

type BrowseProductsResponse = {
  data: BrowseProductsResult;
};

function stableStringify(value: unknown): string {
  if (value === null || value === undefined) return "null";
  if (typeof value !== "object") return JSON.stringify(value);
  if (Array.isArray(value)) {
    return `[${value.map(stableStringify).join(",")}]`;
  }
  const keys = Object.keys(value as Record<string, unknown>).sort();
  const body = keys
    .map(
      (k) =>
        `${JSON.stringify(k)}:${stableStringify((value as Record<string, unknown>)[k])}`,
    )
    .join(",");
  return `{${body}}`;
}

const browseProductsByKey = cache(
  async (key: string): Promise<BrowseProductsResponse> => {
    const vars = JSON.parse(key) as BrowseProductsQueryVariables;
    try {
      // Product grids (homepage, PLP, category listings). Same for everyone; the
      // query vars ride in the fetch body so each filter/sort/page is its own
      // cache entry. Coarse catalog:browse tag → busted on any product / listing
      // change. Dev = always fresh (see surfaceCacheOpts)
      const cacheOpts = await surfaceCacheOpts(CATALOG_SURFACE, {
        tags: ["catalog:browse", "catalog:all"],
      });
      const res = await gqlFetch(BrowseProductsDocument, vars, cacheOpts);
      return { data: res.data.browseProducts };
    } catch {
      // Browse degrades to an empty result on backend / OpenSearch failure so the
      // storefront keeps rendering categories without hard-erroring on a search outage
      return {
        data: { totalCount: 0, items: [], facets: [] },
      };
    }
  },
);

export async function browseProducts(
  vars: BrowseProductsQueryVariables,
): Promise<BrowseProductsResponse> {
  return browseProductsByKey(stableStringify(normalizeBrowseVars(vars)));
}

type BrowseProductsSlimResponse = { data: BrowseProductsSlimQuery["browseProducts"] };

// The items-only slice query behind cumulative load-more rendering - one of these per extra
// page, so it skips facet aggregations and mirrors browseProducts' cache keying and its
// degrade-to-empty posture on a search outage
const browseProductsSlimByKey = cache(
  async (key: string): Promise<BrowseProductsSlimResponse> => {
    const vars = JSON.parse(key) as BrowseProductsQueryVariables;
    try {
      const cacheOpts = await surfaceCacheOpts(CATALOG_SURFACE, {
        tags: ["catalog:browse", "catalog:all"],
      });
      const res = await gqlFetch(BrowseProductsSlimDocument, vars, cacheOpts);
      return { data: res.data.browseProducts };
    } catch {
      return { data: { totalCount: 0, items: [] } };
    }
  },
);

export async function browseProductsSlim(
  vars: BrowseProductsQueryVariables,
): Promise<BrowseProductsSlimResponse> {
  return browseProductsSlimByKey(stableStringify(normalizeBrowseVars(vars)));
}

export type ProductSortOption = ProductSortOptionsQuery["productSortOptions"][number];

export const productSortOptions = cache(
  async (): Promise<ProductSortOption[]> => {
    try {
      const res = await gqlFetch(ProductSortOptionsDocument, {});
      return res.data.productSortOptions;
    } catch {
      return [];
    }
  },
);

export const synapseTrackedSlugs = cache(async (): Promise<Set<string>> => {
  try {
    const res = await gqlFetch(SynapseAttributesDocument, {});
    return new Set(res.data.attributes.filter((a) => a.synapseTrack).map((a) => a.slug));
  } catch {
    return new Set();
  }
});


export const getRecommendations = cache(async (productId: string): Promise<ProductSummary[]> => {
  try {
    const cacheOpts = await surfaceCacheOpts(PRODUCT_SURFACE, {
      tags: ["catalog:product", `catalog:product:${productId}`],
    });
    const res = await gqlFetch(ProductRecommendationsDocument, { id: productId }, cacheOpts);
    const linked = (res.data.product?.linkedProducts ?? []) as unknown as ProductSummary[];
    const also = (res.data.product?.alsoBought ?? []) as unknown as ProductSummary[];
    const seen = new Set<string>();
    const out: ProductSummary[] = [];
    for (const p of [...linked, ...also]) {
      if (seen.has(p.id) || p.id === productId) continue;
      seen.add(p.id);
      out.push(p);
      if (out.length >= 8) break;
    }
    return out;
  } catch {
    return [];
  }
});
export const getCartRecommendations = cache(async (cartId: string): Promise<ProductSummary[]> => {
  try {
    const res = await gqlFetch(CartRecommendationsDocument, { cartId });
    return (res.data.cartRecommendations ?? []) as unknown as ProductSummary[];
  } catch {
    return [];
  }
});
