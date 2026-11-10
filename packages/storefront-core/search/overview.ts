// The AI overview above a search's results: [[n]] in its text stands for the nth of its products

export type OverviewProduct = { id: string; name: string; url: string };
// recap is the backend's own record of what the overview showed, for a chat it's carried into
export type SearchOverview = { text: string; recap: string; products: OverviewProduct[] };

// The Lens events an overview raises, as Signals counts them
export const OVERVIEW_EVENTS = { shown: "search_overview_shown", click: "search_overview_click", chat: "search_overview_chat" } as const;

export type OverviewSegment = { kind: "text"; text: string } | { kind: "product"; product: OverviewProduct; position: number };

const MARKER = /\[\[(\d+)\]\]/g;

export function overviewSegments(text: string, products: OverviewProduct[]): OverviewSegment[] {
  const out: OverviewSegment[] = [];
  let dropped = false;
  const add = (s: string) => {
    if (!s) return;
    const prev = out.at(-1);
    if (prev?.kind !== "text") {
      out.push({ kind: "text", text: s });
      return;
    }
    if (dropped && /\s$/.test(prev.text) && /^[\s.,;:!?]/.test(s)) prev.text = prev.text.trimEnd();
    prev.text += s;
  };
  let last = 0;
  for (const m of text.matchAll(MARKER)) {
    add(text.slice(last, m.index));
    last = m.index + m[0].length;
    const n = Number(m[1]);
    const product = n >= 1 ? products[n - 1] : undefined;
    dropped = !product;
    if (product) out.push({ kind: "product", product, position: n });
  }
  add(text.slice(last));
  const tail = out.at(-1);
  if (tail?.kind === "text") tail.text = tail.text.trimEnd();
  if (tail?.kind === "text" && !tail.text) out.pop();
  return out;
}

export function overviewPlainText(segments: OverviewSegment[]): string {
  return segments.map((s) => (s.kind === "text" ? s.text : s.product.name)).join("").trim();
}

export function overviewNamed(segments: OverviewSegment[], products: OverviewProduct[]): OverviewProduct[] {
  const named = new Map<string, OverviewProduct>();
  for (const s of segments) if (s.kind === "product" && !named.has(s.product.id)) named.set(s.product.id, s.product);
  return named.size > 0 ? [...named.values()] : products;
}

// An overview is only written for a search made on the store itself - typed into its search box or followed from one of
// its pages. A page landed on from elsewhere, or fetched by anything that doesn't say where it came from, never starts one
export function searchedOnSite(headers: Pick<Headers, "get">): boolean {
  const site = headers.get("sec-fetch-site");
  if (site !== null) return site === "same-origin";
  const referer = headers.get("referer");
  const host = (headers.get("x-forwarded-host") ?? headers.get("host") ?? "").split(",")[0].trim();
  if (!referer || !host) return false;
  try {
    return new URL(referer).host === host;
  } catch {
    return false;
  }
}

export type OverviewExchange = { question: string; reply: string; recap: string; productIds: string[] };

// The recap is the backend's own, so a later "the first one" in the chat still knows which product
export function overviewExchange(query: string, overview: SearchOverview): OverviewExchange {
  const segments = overviewSegments(overview.text, overview.products);
  const named = overviewNamed(segments, overview.products);
  return {
    question: query.trim(),
    reply: overviewPlainText(segments),
    recap: overview.recap,
    productIds: named.map((p) => p.id),
  };
}
