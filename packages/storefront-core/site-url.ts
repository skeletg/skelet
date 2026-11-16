// The storefront's public origin - the single base every absolute URL (sitemaps, JSON-LD,
// canonicals, og tags) is built from, so two features can't disagree about the host

export const SITE_URL = trimOrigin(process.env.STOREFRONT_BASE_URL ?? "http://localhost:3000");

// The host a request was made to, as the proxy in front forwarded it - the first of a list, since each hop may add one
export function requestHost(headers: Pick<Headers, "get">): string {
  return (headers.get("x-forwarded-host") ?? headers.get("host") ?? "").split(",")[0].trim();
}

export function trimOrigin(url: string): string {
  return url.trim().replace(/\/+$/, "");
}

export function absUrl(path: string, base: string = SITE_URL): string {
  // operator extras may already be absolute - don't double-prefix them
  if (/^https?:\/\//.test(path)) return path;
  return base + (path.startsWith("/") ? path : "/" + path);
}
