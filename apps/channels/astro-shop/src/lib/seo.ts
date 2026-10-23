// Pure SEO helpers - absolute URLs and schema.org JSON-LD, unit-tested without a backend

// The channel's public URL is the canonical origin; a dev or preview host falls back to the request's
export function siteOrigin(publicUrl: string | null | undefined, requestOrigin: string): string {
  try {
    if (publicUrl) return new URL(publicUrl).origin;
  } catch {
    // a malformed public URL - the request's origin is still a real one
  }
  return requestOrigin;
}

export function absolute(origin: string, path: string): string {
  return new URL(path.startsWith("/") ? path : `/${path}`, origin).toString();
}

// JSON-LD sits inside <script> - a "</script>" in a product name must not end it early
export function jsonLd(data: unknown): string {
  return JSON.stringify(data).replace(/</g, "\\u003c");
}

export function websiteLd(name: string, origin: string) {
  return { "@context": "https://schema.org", "@type": "WebSite", name, url: absolute(origin, "/") };
}

type LdProduct = {
  name: string;
  sku: string;
  url: string;
  description?: string;
  image?: string | null;
  price: number;
  inStock: boolean;
};

export function productLd(p: LdProduct, currency: { code: string; decimals?: number | null }, brand: string) {
  const decimals = currency.decimals ?? 2;
  return {
    "@context": "https://schema.org",
    "@type": "Product",
    name: p.name,
    sku: p.sku,
    url: p.url,
    ...(p.description ? { description: p.description } : {}),
    ...(p.image ? { image: [p.image] } : {}),
    brand: { "@type": "Brand", name: brand },
    offers: {
      "@type": "Offer",
      url: p.url,
      priceCurrency: currency.code,
      // schema.org wants the major-unit amount as a plain number string, not a formatted price
      price: (p.price / 10 ** decimals).toFixed(decimals),
      availability: p.inStock ? "https://schema.org/InStock" : "https://schema.org/OutOfStock",
      itemCondition: "https://schema.org/NewCondition",
    },
  };
}

export function breadcrumbLd(items: { name: string; url: string }[]) {
  return {
    "@context": "https://schema.org",
    "@type": "BreadcrumbList",
    itemListElement: items.map((it, i) => ({ "@type": "ListItem", position: i + 1, name: it.name, item: it.url })),
  };
}

// Meta descriptions read best under ~160 characters, cut on a word
export function clip(text: string, max = 160): string {
  const t = text.replace(/\s+/g, " ").trim();
  if (t.length <= max) return t;
  const cut = t.slice(0, max - 1);
  return `${cut.slice(0, cut.lastIndexOf(" ") > 80 ? cut.lastIndexOf(" ") : cut.length)}…`;
}

export function sitemapXml(urls: string[]): string {
  const esc = (s: string) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  const body = urls.map((u) => `  <url><loc>${esc(u)}</loc></url>`).join("\n");
  return `<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n${body}\n</urlset>\n`;
}
