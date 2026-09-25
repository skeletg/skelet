// Maps a browser path onto /<channel>/<locale>/ so each channel and language caches its own render - no imports, the proxy runs it

// Segments that keep their exact path: the backend proxies and the root route handlers external links point at
export const NOT_LOCALIZED = new Set(["api", "t", "download", "unsubscribe", "e", "coupon", "masquerade"]);

// Pages never carry a file extension, and the root-level files that do (sitemaps, robots.txt) are not per-locale routes
export function isPagePath(pathname: string): boolean {
  const segs = pathname.split("/");
  if (NOT_LOCALIZED.has(segs[1] ?? "")) return false;
  return !(segs[segs.length - 1] ?? "").includes(".");
}

const CHANNEL_SEGMENT = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

// The /<channel>/ tree is only reachable through the proxy's rewrite, so a browser naming it directly is refused
export function isChannelTreePath(pathname: string): boolean {
  return CHANNEL_SEGMENT.test(pathname.split("/")[1] ?? "");
}

export function isLegacyMatched(pathname: string): boolean {
  return !pathname.startsWith("/_next") && !pathname.startsWith("/api") && !pathname.includes(".");
}

// null when the path is not a page. With no locale list yet the path goes in as-is, so the layout's guard decides
export function internalPath(
  pathname: string,
  channelId: string,
  codes: string[],
  defaultLocale: string,
): string | null {
  if (!isPagePath(pathname)) return null;
  const rest = pathname === "/" ? "" : pathname;
  const first = pathname.split("/")[1] ?? "";
  if (!defaultLocale || codes.includes(first)) return `/${channelId}${rest}`;
  return `/${channelId}/${defaultLocale}${rest}`;
}

export function channelRoutePaths(path: string, channelId: string, codes: string[]): string[] {
  const p = path.startsWith("/") ? path : `/${path}`;
  const first = p.split("/")[1] ?? "";
  if (codes.includes(first)) return [`/${channelId}${p}`];
  const rest = p === "/" ? "" : p;
  return codes.map((c) => `/${channelId}/${c}${rest}`);
}
