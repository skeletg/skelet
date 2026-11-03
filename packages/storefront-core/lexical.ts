// Lexical documents as the dashboard writes them: the node types, the text-format flags, and the reads a page needs
// without drawing anything. Unknown node types are skipped, not thrown - a new dashboard node must not crash us
//
// SYNC SENTINEL - node types must match
// apps/dashboard/src/features/catalog/lexical-blocks/*.tsx

export type LexicalRoot = {
  type: "root";
  children: LexicalNode[];
};

// Paragraph children can include block-level custom nodes when Lexical's $insertNodes ran inside a range selection - the FAQ /
// product-image / structured-block decorators get nested inside the current paragraph instead of sitting as root siblings
export type Paragraph = {
  type: "paragraph";
  children: (InlineNode | BlockNode)[];
};

export type Heading = {
  type: "heading";
  tag: "h1" | "h2" | "h3" | "h4" | "h5" | "h6";
  children: InlineNode[];
};

export type ListNode = {
  type: "list";
  listType: "bullet" | "number" | "check";
  tag: "ul" | "ol";
  start?: number;
  children: ListItem[];
};

export type ListItem = {
  type: "listitem";
  value?: number;
  checked?: boolean;
  children: (InlineNode | ListNode)[];
};

export type Quote = {
  type: "quote";
  children: InlineNode[];
};

export type Link = {
  type: "link";
  url: string;
  rel?: string | null;
  target?: string | null;
  title?: string | null;
  children: InlineNode[];
};

export type Text = {
  type: "text";
  text: string;
  format?: number;
  style?: string;
};

export type Linebreak = {
  type: "linebreak";
};

export type InlineNode = Text | Link | Linebreak;

// ===== Custom block types - MUST match dashboard exports =====

export type ProductImage = {
  type: "product-image";
  src: string;
  altText: string;
  width: number;
  height: number;
  version: 1;
};

export type FAQItem = { question: string; answer: string };

export type FAQBlock = {
  type: "faq-block";
  items: FAQItem[];
  version: 1;
};

export type StructuredBlock = {
  type: "structured-block";
  blockType: StructuredBlockType;
  data: StructuredBlockData;
  version: 1;
};

export type StructuredBlockType =
  | "pros-cons"
  | "comparison-table"
  | "tabs"
  | "testimonials"
  | "video-embed"
  | "specs-table"
  | "image-text";

export type StructuredBlockData =
  | ProsConsData
  | ComparisonTableData
  | TabsData
  | TestimonialsData
  | VideoEmbedData
  | SpecsTableData
  | ImageTextData;

export type ProsConsData = { pros: string[]; cons: string[] };
export type ComparisonTableData = { columns: string[]; rows: string[][] };
export type TabsData = { tabs: { label: string; content: string }[] };
export type TestimonialsData = {
  items: { quote: string; author: string; role: string }[];
};
export type VideoEmbedData = { title: string; url: string };
export type SpecsTableData = { rows: { label: string; value: string }[] };
export type ImageTextData = {
  imageUrl: string;
  imageAlt: string;
  title: string;
  body: string;
  layout: "image-left" | "image-right";
};

export type BlockNode =
  | Paragraph
  | Heading
  | ListNode
  | Quote
  | ProductImage
  | FAQBlock
  | StructuredBlock;

export type LexicalNode = BlockNode | InlineNode | ListItem;

export type Envelope = { root: LexicalRoot };

export const IS_BOLD = 1;
export const IS_ITALIC = 2;
export const IS_STRIKETHROUGH = 4;
export const IS_UNDERLINE = 8;
export const IS_CODE = 16;
export const IS_SUBSCRIPT = 32;
export const IS_SUPERSCRIPT = 64;
export const IS_HIGHLIGHT = 128;

/**
 * lexicalHasContent returns true if the value parses into at least one
 * renderable block. Counts custom blocks (faq, product-image,
 * structured-*) - they have no `text` leaves but ARE content. Use this
 * instead of `lexicalPlainText(...).length > 0` to gate "is there a body
 * to render?"; that helper only sees text and would hide a body composed
 * entirely of custom blocks.
 */
export function lexicalHasContent(value: unknown): boolean {
  return lexicalBlocks(value).length > 0;
}

/**
 * lexicalPlainText is a low-fidelity extraction for places that need
 * a single string (meta descriptions, search-index excerpts, etc.).
 * Walks every text leaf and joins with spaces. Returns "" for a body
 * made entirely of custom blocks - use lexicalHasContent if you're
 * asking "is there anything to render."
 */
export function lexicalPlainText(value: unknown): string {
  const blocks = lexicalBlocks(value);
  const parts: string[] = [];
  function walk(node: LexicalNode) {
    if (node.type === "text") {
      const s = node.text.trim();
      if (s) parts.push(s);
      return;
    }
    if ("children" in node && Array.isArray(node.children)) {
      for (const c of node.children) walk(c);
    }
  }
  for (const b of blocks) walk(b);
  return parts.join(" ");
}

// Plain text cut to meta-description length, on a word boundary - the fallback when an
// operator wrote no meta description, so the tag carries the page's own copy instead of
// the channel-wide default. Search engines truncate around 155-160 characters anyway
export function lexicalSummary(value: unknown, max = 160): string {
  const text = lexicalPlainText(value).replace(/\s+/g, " ");
  if (text.length <= max) return text;
  const cut = text.slice(0, max - 1);
  const space = cut.lastIndexOf(" ");
  return `${(space > max / 2 ? cut.slice(0, space) : cut).replace(/[\s.,;:!?-]+$/, "")}…`;
}

// lexicalBlocks is the document's top-level blocks: a plain string becomes one paragraph, anything unreadable none
export function lexicalBlocks(value: unknown): BlockNode[] {
  if (value == null) return [];
  if (typeof value === "string") {
    const s = value.trim();
    if (!s) return [];
    // Plain string fallback for seeded or freeform data - wrap as a single paragraph so
    // renderers downstream see a structured shape
    return [
      {
        type: "paragraph",
        children: [{ type: "text", text: s }],
      },
    ];
  }
  if (typeof value !== "object") return [];
  const env = value as Partial<Envelope>;
  if (!env.root || typeof env.root !== "object") return [];
  const children = env.root.children;
  if (!Array.isArray(children)) return [];
  return children.filter(isBlock);
}

function isBlock(n: unknown): n is BlockNode {
  if (!n || typeof n !== "object") return false;
  const t = (n as { type?: unknown }).type;
  return (
    t === "paragraph" ||
    t === "heading" ||
    t === "list" ||
    t === "quote" ||
    t === "product-image" ||
    t === "faq-block" ||
    t === "structured-block"
  );
}
