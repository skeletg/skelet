// The storefront's public origin - the single base every absolute URL (sitemaps, JSON-LD,
// canonicals, og tags) is built from, so two features can't disagree about the host

export const SITE_URL = (
  process.env.STOREFRONT_BASE_URL ?? "http://localhost:3000"
).replace(/\/$/, "");

export function absUrl(path: string): string {
  // operator extras may already be absolute - don't double-prefix them
  if (/^https?:\/\//.test(path)) return path;
  return SITE_URL + (path.startsWith("/") ? path : "/" + path);
}
