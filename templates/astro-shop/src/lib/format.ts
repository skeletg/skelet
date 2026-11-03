import { formatPrice, type CurrencyInfo } from "@skelet/storefront-core/format";

export type Currency = CurrencyInfo & { locale?: string };

export function money(minor: number, c: Currency): string {
  return formatPrice(minor, c, c.locale);
}

type RichNode = { type?: string; text?: string; children?: RichNode[] };

// Product descriptions are Lexical JSON - this shop shows them as plain paragraphs
// shortcut: drops headings, lists and links' styling, render the full Lexical tree if copy needs it
export function paragraphs(doc: unknown): string[] {
  const root = (doc as { root?: RichNode } | null)?.root;
  const text = (n: RichNode): string => n.text ?? (n.children ?? []).map(text).join("");
  return (root?.children ?? []).map(text).map((s) => s.trim()).filter(Boolean);
}
