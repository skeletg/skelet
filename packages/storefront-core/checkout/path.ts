// Offer identity stays in this tab's URL; the browser-wide cookie only carries access.
export function checkoutHref(offerId?: string, extra: Record<string, string> = {}): string {
  const query = new URLSearchParams(extra);
  if (offerId) query.set("offer", offerId);
  return `/checkout${query.size ? `?${query}` : ""}`;
}
