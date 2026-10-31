// Pure helpers - no imports, so they unit-test without a backend

export type Currency = { code: string; symbol?: string | null; symbolPosition?: string | null; decimals?: number | null };

// Amounts arrive in minor units. The backend's per-currency symbol and position win over the
// locale's, and with no symbol configured Intl's own currency style answers
export function money(minor: number, c: Currency, locale = "en"): string {
  const decimals = c.decimals ?? 2;
  const n = minor / 10 ** decimals;
  const digits = { minimumFractionDigits: decimals, maximumFractionDigits: decimals };
  if (!c.symbol) return new Intl.NumberFormat(locale, { style: "currency", currency: c.code || "USD", ...digits }).format(n);
  const amount = new Intl.NumberFormat(locale, digits).format(n);
  return c.symbolPosition === "suffix" ? `${amount} ${c.symbol}` : `${c.symbol}${amount}`;
}

type RichNode = { type?: string; text?: string; children?: RichNode[] };

// Product descriptions are Lexical JSON - this shop shows them as plain paragraphs
// shortcut: drops headings, lists and links' styling, render the full Lexical tree if copy needs it
export function paragraphs(doc: unknown): string[] {
  const root = (doc as { root?: RichNode } | null)?.root;
  const text = (n: RichNode): string => n.text ?? (n.children ?? []).map(text).join("");
  return (root?.children ?? []).map(text).map((s) => s.trim()).filter(Boolean);
}
