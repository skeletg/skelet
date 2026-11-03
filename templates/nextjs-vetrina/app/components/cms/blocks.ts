// Wire shape of a component-built page's content.body
//
// SYNC SENTINEL - contract shared three ways with
// backend internal/blockkit and the dashboard features/cms descriptors
// A body that doesn't parse to a Document is treated as freeform content - one that claims to be a Document (a blocks key) but
// isn't well-formed is refused outright, matching the backend, which drops such a body before it ships

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

const MAX_DEPTH = 8;

// Any malformed node rejects the whole document rather than being dropped, so a body the backend never sanitized can't reach a renderer
export function parseDocument(body: unknown): Document | null {
  if (!isPlainObject(body)) return null;
  if (!Number.isInteger(body.version) || (body.version as number) < 0) return null;
  const blocks = parseBlocks(body.blocks, 1);
  if (!blocks) return null;
  return { version: body.version as number, blocks };
}

function parseBlocks(value: unknown, depth: number): Block[] | null {
  if (!Array.isArray(value) || depth > MAX_DEPTH) return null;
  const out: Block[] = [];
  for (const node of value) {
    const block = parseBlock(node, depth);
    if (!block) return null;
    out.push(block);
  }
  return out;
}

function parseBlock(value: unknown, depth: number): Block | null {
  if (!isPlainObject(value)) return null;
  const { id, type, props, blocks } = value;
  if (typeof id !== "string" || id === "" || typeof type !== "string" || type === "") return null;
  if (props !== undefined && props !== null && !isPlainObject(props)) return null;
  const block: Block = { id, type, props: (props as Record<string, unknown> | null | undefined) ?? {} };
  if (blocks !== undefined && blocks !== null) {
    const children = parseBlocks(blocks, depth + 1);
    if (!children) return null;
    block.blocks = children;
  }
  return block;
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
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
