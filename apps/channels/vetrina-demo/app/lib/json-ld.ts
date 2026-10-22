// Schema.org structured-data builders
//
// Every href these builders receive must already be the LOCALE-CANONICAL path (what
// localizedAlternates calls canonical) - run page paths through canonicalPathMapper first, so the
// structured data names the same URL the <link rel=canonical> does. abs() only adds the origin

import { displayName } from "@/app/lib/display";
import { currencyDecimals, toMajor, type CurrencyInfo } from "@/app/lib/format";
import { lexicalPlainText } from "@/app/lib/lexical-render";
import { SITE_URL, absUrl as abs } from "@/app/lib/site-url";

type ProductLikeForJsonLd = {
  id: string;
  url: string;
  sku: string;
  name?: string | null;
  kind?: string;
  defaultPrice: number;
  defaultCurrencyCode: string;
  inStock: boolean;
  shortDescription?: unknown;
  description?: unknown;
  metaTitle?: string | null;
  metaDescription?: string | null;
  mainImage?: { url: string } | null;
  media?: { url: string }[];
  // The configurable's purchasable variants - their price spread becomes the AggregateOffer
  children?: { defaultPrice: number; inStock: boolean }[];
  attributeValues?: {
    attribute: { slug: string };
    selectedValues: { name?: string | null; value: string }[];
    scalarValue?: unknown;
  }[];
};

// Product identity fields live in ordinary catalog attributes - these slugs are the convention
// the seed data and docs use, and a store that names them differently just doesn't get the field
const BRAND_ATTRIBUTES = ["brand", "manufacturer"];
const GTIN_ATTRIBUTES = ["gtin", "gtin13", "ean", "upc", "barcode"];
const MPN_ATTRIBUTES = ["mpn"];

function attributeText(
  product: ProductLikeForJsonLd,
  slugs: string[],
): string | undefined {
  for (const slug of slugs) {
    const av = product.attributeValues?.find((a) => a.attribute.slug === slug);
    if (!av) continue;
    const selected = av.selectedValues
      .map((s) => s.name || s.value)
      .filter(Boolean)
      .join(", ");
    if (selected) return selected;
    if (typeof av.scalarValue === "string" && av.scalarValue.trim()) return av.scalarValue;
    if (typeof av.scalarValue === "number") return String(av.scalarValue);
  }
  return undefined;
}

// Maps the backend `inStock` flag to schema.org's enumerated value
function availability(inStock: boolean): string {
  return inStock
    ? "https://schema.org/InStock"
    : "https://schema.org/OutOfStock";
}

type ReviewsForJsonLd = {
  count: number;
  overallAverage?: number | null;
  reviews: {
    customerDisplayName: string;
    title?: string | null;
    body?: string | null;
    submittedAt: string;
    overallRating?: number | null;
  }[];
};

export function productJsonLd(
  product: ProductLikeForJsonLd,
  href: string,
  reviewsData?: ReviewsForJsonLd,
  base: string = SITE_URL,
  // The store's configured currency (priceCurrency) - the bare code formats at ISO precision
  currency: string | CurrencyInfo = product.defaultCurrencyCode,
): Record<string, unknown> {
  // Media URLs come back host-relative - a relative image is invisible to a crawler
  // reading the JSON out of context
  const images: string[] = [];
  if (product.mainImage?.url) images.push(abs(product.mainImage.url, base));
  for (const m of product.media ?? []) {
    const u = m.url ? abs(m.url, base) : "";
    if (u && !images.includes(u)) images.push(u);
  }

  // Prefer the operator-authored meta description, then the short description, then the
  // long description - same precedence the page chrome uses for SEO copy
  const description =
    product.metaDescription ||
    lexicalPlainText(product.shortDescription) ||
    lexicalPlainText(product.description) ||
    undefined;

  const amount = (minor: number) => toMajor(minor, currency).toFixed(currencyDecimals(currency));
  const price = amount(product.defaultPrice);

  // A configurable's own defaultPrice is just the anchor - what a shopper can buy are the
  // variants, so the offer is the aggregate of their prices, in stock if any of them is
  const variants = (product.children ?? []).filter((c) => typeof c.defaultPrice === "number");
  const offers =
    product.kind === "CONFIGURABLE" && variants.length > 0
      ? {
          "@type": "AggregateOffer",
          url: abs(href, base),
          priceCurrency: product.defaultCurrencyCode,
          lowPrice: amount(Math.min(...variants.map((c) => c.defaultPrice))),
          highPrice: amount(Math.max(...variants.map((c) => c.defaultPrice))),
          offerCount: variants.length,
          availability: availability(variants.some((c) => c.inStock)),
          itemCondition: "https://schema.org/NewCondition",
        }
      : {
          "@type": "Offer",
          url: abs(href, base),
          priceCurrency: product.defaultCurrencyCode,
          price,
          availability: availability(product.inStock),
          itemCondition: "https://schema.org/NewCondition",
        };

  const brand = attributeText(product, BRAND_ATTRIBUTES);
  const gtin = attributeText(product, GTIN_ATTRIBUTES);
  const mpn = attributeText(product, MPN_ATTRIBUTES);

  const out: Record<string, unknown> = {
    "@context": "https://schema.org",
    "@type": "Product",
    "@id": abs(href, base),
    name: displayName(product.name, product.sku),
    sku: product.sku,
    productID: product.id,
    url: abs(href, base),
    image: images.length > 0 ? images : undefined,
    description,
    brand: brand ? { "@type": "Brand", name: brand } : undefined,
    gtin,
    mpn,
    offers,
  };

  if (
    reviewsData &&
    reviewsData.count > 0 &&
    typeof reviewsData.overallAverage === "number"
  ) {
    out.aggregateRating = {
      "@type": "AggregateRating",
      ratingValue: Number(reviewsData.overallAverage.toFixed(2)),
      reviewCount: reviewsData.count,
      bestRating: 5,
      worstRating: 1,
    };
    const reviewItems = reviewsData.reviews
      .filter((r) => typeof r.overallRating === "number")
      .slice(0, 10)
      .map((r) => ({
        "@type": "Review",
        author: {
          "@type": "Person",
          name: r.customerDisplayName || "Customer",
        },
        datePublished: r.submittedAt,
        name: r.title || undefined,
        reviewBody: r.body || undefined,
        reviewRating: {
          "@type": "Rating",
          ratingValue: r.overallRating,
          bestRating: 5,
          worstRating: 1,
        },
      }));
    if (reviewItems.length > 0) {
      out.review = reviewItems;
    }
  }

  return out;
}

type ItemListProduct = {
  url: string;
  sku: string;
  name?: string | null;
};

// The PLP schema: an ItemList of the products visible on this page, positions continuing across
// pagination so page 2 doesn't restart at 1. Summary-page style on purpose - each ListItem is
// just a pointer at the PDP, whose own Product markup is the source of truth for price and stock
export function itemListJsonLd(
  products: ItemListProduct[],
  toHref: (p: ItemListProduct) => string,
  offset = 0,
  base: string = SITE_URL,
): Record<string, unknown> {
  return {
    "@context": "https://schema.org",
    "@type": "ItemList",
    numberOfItems: products.length,
    itemListElement: products.map((p, i) => ({
      "@type": "ListItem",
      position: offset + i + 1,
      name: displayName(p.name, p.sku),
      url: abs(toHref(p), base),
    })),
  };
}

type BreadcrumbItem = {
  name: string;
  href: string;
};

export function breadcrumbJsonLd(
  items: BreadcrumbItem[],
  base: string = SITE_URL,
): Record<string, unknown> {
  return {
    "@context": "https://schema.org",
    "@type": "BreadcrumbList",
    itemListElement: items.map((it, i) => ({
      "@type": "ListItem",
      position: i + 1,
      name: it.name,
      item: abs(it.href, base),
    })),
  };
}

type BlogPostForJsonLd = {
  slug: string;
  publishedAt: string | null;
  author: { displayName: string } | null;
  content: {
    title: string;
    metaTitle?: string | null;
    metaDescription?: string | null;
    excerpt?: unknown;
  } | null;
};

export function blogPostingJsonLd(
  post: BlogPostForJsonLd,
  href: string,
  base: string = SITE_URL,
): Record<string, unknown> {
  const title = displayName(
    post.content?.metaTitle || post.content?.title,
    post.slug,
  );
  const description =
    post.content?.metaDescription ||
    lexicalPlainText(post.content?.excerpt) ||
    undefined;
  return {
    "@context": "https://schema.org",
    "@type": "BlogPosting",
    "@id": abs(href, base),
    headline: title,
    url: abs(href, base),
    datePublished: post.publishedAt ?? undefined,
    dateModified: post.publishedAt ?? undefined,
    description,
    author: post.author
      ? {
          "@type": "Person",
          name: post.author.displayName,
        }
      : undefined,
  };
}

type CMSPageForJsonLd = {
  slug: string;
  publishedAt: string | null;
  content: {
    title: string;
    metaTitle?: string | null;
    metaDescription?: string | null;
  } | null;
};

export function cmsPageJsonLd(
  page: CMSPageForJsonLd,
  href: string,
  base: string = SITE_URL,
): Record<string, unknown> {
  const title = displayName(
    page.content?.metaTitle || page.content?.title,
    page.slug,
  );
  return {
    "@context": "https://schema.org",
    "@type": "WebPage",
    "@id": abs(href, base),
    name: title,
    url: abs(href, base),
    description: page.content?.metaDescription || undefined,
    datePublished: page.publishedAt ?? undefined,
    dateModified: page.publishedAt ?? undefined,
  };
}

// The builder spec the dashboard stores in seo.schema - a type plus toggles for which page-derived fields to include
// SYNC: mirrored by buildSchemaFromSpec in apps/dashboard/src/features/cms/schema-jsonld.ts
export type CmsSchemaSpec = {
  type?: "WebPage" | "Article";
  image?: boolean;
  datePublished?: boolean;
  dateModified?: boolean;
  author?: string;
};

type CMSPageForSchemaSpec = {
  slug: string;
  publishedAt: string | null;
  canonicalUrl: string | null;
  ogImage: string | null;
  content: {
    title: string;
    metaTitle?: string | null;
    metaDescription?: string | null;
  } | null;
};

// Generates JSON-LD from the operator's builder spec + live page fields - the "select a few things from the page" path, built fresh at
// render so a later title/URL/image edit flows through - returns null on an unparseable spec so a bad value can't break the page
export function cmsSchemaFromSpec(
  raw: string,
  page: CMSPageForSchemaSpec,
  href: string,
  base: string = SITE_URL,
): Record<string, unknown> | null {
  let spec: CmsSchemaSpec;
  try {
    spec = JSON.parse(raw) as CmsSchemaSpec;
  } catch {
    return null;
  }
  if (!spec || typeof spec !== "object") return null;

  const name = displayName(page.content?.metaTitle || page.content?.title, page.slug);
  const url = abs(page.canonicalUrl?.trim() || href, base);
  const description = page.content?.metaDescription || undefined;
  const image = spec.image && page.ogImage ? abs(page.ogImage, base) : undefined;
  // publishedAt is the only date the page carries, so it stands in for both when toggled
  const datePublished = spec.datePublished ? page.publishedAt ?? undefined : undefined;
  const dateModified = spec.dateModified ? page.publishedAt ?? undefined : undefined;

  if (spec.type === "Article") {
    const author = spec.author?.trim();
    return {
      "@context": "https://schema.org",
      "@type": "Article",
      "@id": url,
      headline: name,
      url,
      description,
      image,
      datePublished,
      dateModified,
      author: author ? { "@type": "Person", name: author } : undefined,
    };
  }
  return {
    "@context": "https://schema.org",
    "@type": "WebPage",
    "@id": url,
    name,
    url,
    description,
    image,
    datePublished,
    dateModified,
  };
}

// Operator JSON-LD arrives as a raw string - parse it so JsonLd can re-serialize +
// escape it, and an invalid or blank value becomes null instead of a broken script tag
export function parseCustomJsonLd(raw: string | null): unknown {
  if (!raw || !raw.trim()) return null;
  try {
    return JSON.parse(raw);
  } catch {
    return null;
  }
}

export function organizationJsonLd(
  input: {
    name: string;
    email?: string | null;
  },
  base: string = SITE_URL,
): Record<string, unknown> {
  return {
    "@context": "https://schema.org",
    "@type": "Organization",
    "@id": abs("/", base) + "#organization",
    name: input.name,
    url: abs("/", base),
    ...(input.email ? { email: input.email } : {}),
  };
}

export function webSiteJsonLd(input: { name: string }, base: string = SITE_URL): Record<string, unknown> {
  return {
    "@context": "https://schema.org",
    "@type": "WebSite",
    "@id": abs("/", base) + "#website",
    name: input.name,
    url: abs("/", base),
    publisher: { "@id": abs("/", base) + "#organization" },
    potentialAction: {
      "@type": "SearchAction",
      target: {
        "@type": "EntryPoint",
        urlTemplate: abs("/search", base) + "?q={search_term_string}",
      },
      "query-input": "required name=search_term_string",
    },
  };
}
