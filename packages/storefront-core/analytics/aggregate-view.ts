// A page view from a visitor who hasn't allowed analytics, counted by the proxy as nobody - no cookie read or set, no
// browser script, only the path, so it adds to Signals' totals and never to a person. A visitor who did allow analytics is
// counted by Lens instead, so nobody is counted twice. No Next imports, so the edge proxy can load it

export const COUNT_PATH = "/api/front/count";

// A real look at a page: a GET that isn't Next fetching ahead of a click. An RSC request without a prefetch marker is a
// client-side navigation, which is a view
// shortcut: a router.refresh() after a server action reads as a view too - mark it if those inflate the counts
export function countableView(method: string, headers: Headers, analytics: boolean, isBot: boolean): boolean {
  if (analytics || isBot || method !== "GET") return false;
  if (headers.has("next-router-prefetch")) return false;
  const purpose = headers.get("sec-purpose") ?? headers.get("purpose") ?? "";
  return !purpose.includes("prefetch");
}
