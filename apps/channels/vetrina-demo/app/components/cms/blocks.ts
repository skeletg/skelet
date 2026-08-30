// Wire shape of a component-built page's content.body
//
// SYNC SENTINEL - contract shared three ways with
// backend internal/blockkit and the dashboard features/cms descriptors
// A body that doesn't parse to a Document is treated as freeform content

export type Block = {
  id: string;
  type: string;
  props: Record<string, unknown>;
  blocks?: Block[];
};

export type Document = {
  version: number;
  blocks: Block[];
};

export function parseDocument(body: unknown): Document | null {
  if (!body || typeof body !== "object" || Array.isArray(body)) return null;
  const obj = body as Record<string, unknown>;
  const blocks = obj.blocks;
  if (!Array.isArray(blocks)) return null;
  // Keep only well-formed block nodes; a malformed node is dropped rather than failing
  // the whole document, so one bad block can't blank the page
  const normalized = (blocks.filter(isBlock) as Block[]).map((b) =>
    b.props && typeof b.props === "object" ? b : { ...b, props: {} },
  );
  const version = typeof obj.version === "number" ? obj.version : 1;
  return { version, blocks: normalized };
}

function isBlock(value: unknown): value is Block {
  if (!value || typeof value !== "object") return false;
  const b = value as Record<string, unknown>;
  return typeof b.id === "string" && typeof b.type === "string";
}

export function readString(
  props: Record<string, unknown>,
  key: string,
): string | undefined {
  const v = props[key];
  return typeof v === "string" && v.length > 0 ? v : undefined;
}

export function readNumber(
  props: Record<string, unknown>,
  key: string,
): number | undefined {
  const v = props[key];
  return typeof v === "number" && Number.isFinite(v) ? v : undefined;
}
export type ImageValue = {
  src: string;
  alt: string;
  width?: number;
  height?: number;
  focalX?: number;
  focalY?: number;
};

export function readImage(
  props: Record<string, unknown>,
  key: string,
): ImageValue | undefined {
  const v = props[key];
  if (typeof v === "string") return v.length > 0 ? { src: v, alt: "" } : undefined;
  if (v && typeof v === "object" && !Array.isArray(v)) {
    const o = v as Record<string, unknown>;
    const src = typeof o.src === "string" ? o.src : "";
    if (!src) return undefined;
    return {
      src,
      alt: typeof o.alt === "string" ? o.alt : "",
      width: typeof o.width === "number" ? o.width : undefined,
      height: typeof o.height === "number" ? o.height : undefined,
      focalX: typeof o.focalX === "number" ? o.focalX : undefined,
      focalY: typeof o.focalY === "number" ? o.focalY : undefined,
    };
  }
  return undefined;
}

export function readItems(
  props: Record<string, unknown>,
  key: string,
): Array<Record<string, unknown>> {
  const v = props[key];
  if (!Array.isArray(v)) return [];
  return v.filter(
    (x): x is Record<string, unknown> => !!x && typeof x === "object" && !Array.isArray(x),
  );
}
