const PRIVATE_PAGES = new Set(["account", "cart", "checkout", "orders", "wishlist", "verify-email", "handoff", "offer", "gift-card", "track-order", "coupon", "download", "subscribe", "unsubscribe", "masquerade"]);
const CHANNEL = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const LOCALE = /^[a-z]{2,3}(-[a-z0-9]{2,8})*$/;

export function isStorePath(path: string): boolean {
  return /^\/(?!\/)/.test(path) && !/[\\\x00-\x1f\x7f]/.test(path);
}

function segments(pathname: string): string[] | null {
  try {
    return pathname.split("/").filter(Boolean).map((s) => decodeURIComponent(s).toLowerCase());
  } catch {
    return null;
  }
}

export function pageForAssistant(pathname: string, search: string): string {
  const parts = isStorePath(pathname) ? segments(pathname) : null;
  if (!parts) return "";
  const first = CHANNEL.test(parts[0] ?? "") ? 1 : 0;
  if (PRIVATE_PAGES.has(parts[first]) || (LOCALE.test(parts[first] ?? "") && PRIVATE_PAGES.has(parts[first + 1]))) return "";
  const q = new URLSearchParams(search).get("q");
  return q ? `${pathname}?${new URLSearchParams({ q })}` : pathname;
}
