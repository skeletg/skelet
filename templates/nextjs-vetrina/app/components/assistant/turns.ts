import { isStorePath } from "@skelet/storefront-core/assistant/page";
import type { AssistantDetail, AssistantLink, AssistantMessage } from "@skelet/storefront-core/assistant/types";
import type { ProductSummary } from "@skelet/storefront-next/catalog/read";

export type Turn = {
  role: "user" | "assistant";
  content: string;
  recap?: string;
  followUps?: string[];
  detail?: AssistantDetail | null;
  links?: AssistantLink[];
  navigated?: AssistantLink | null;
  products?: ProductSummary[];
  showCart?: boolean;
  incomplete?: boolean;
  cartLost?: boolean;
  cartMissed?: string[];
};

// Per tab, and versioned with the turn's shape so a tab holding an older one starts over
export const CONVERSATION_KEY = "assistant:conversation:v2";

const str = (v: unknown): v is string => typeof v === "string";
const record = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);
const strings = (v: unknown): v is string[] => Array.isArray(v) && v.every(str);

function isLink(v: unknown): v is AssistantLink {
  return (
    record(v) &&
    str(v.label) &&
    str(v.path) &&
    isStorePath(v.path) &&
    str(v.query) &&
    str(v.sort) &&
    (v.count === null || typeof v.count === "number") &&
    Array.isArray(v.filters) &&
    v.filters.every((f) => record(f) && str(f.slug) && strings(f.values))
  );
}

function isProduct(v: unknown): v is ProductSummary {
  return record(v) && str(v.id) && str(v.name) && str(v.kind) && typeof v.defaultPrice === "number";
}

function isDetail(v: unknown): v is AssistantDetail {
  if (!record(v) || !isProduct(v.product) || !Array.isArray(v.specs) || !record(v.notes)) return false;
  const n = v.notes;
  return str(n.summary) && strings(n.goodFor) && strings(n.keepInMind) && Array.isArray(n.highlights) && n.highlights.every((h) => record(h) && str(h.title) && str(h.detail));
}

export function readTurn(v: unknown): Turn | null {
  if (!record(v) || (v.role !== "user" && v.role !== "assistant") || !str(v.content)) return null;
  if (v.role === "user") return { role: "user", content: v.content };
  const turn: Turn = { role: "assistant", content: v.content };
  if (str(v.recap)) turn.recap = v.recap;
  if (strings(v.followUps)) turn.followUps = v.followUps;
  if (isDetail(v.detail)) turn.detail = v.detail;
  if (Array.isArray(v.links) && v.links.every(isLink)) turn.links = v.links;
  if (isLink(v.navigated)) turn.navigated = v.navigated;
  if (Array.isArray(v.products) && v.products.every(isProduct)) turn.products = v.products;
  if (v.showCart === true) turn.showCart = true;
  if (v.incomplete === true) turn.incomplete = true;
  if (v.cartLost === true) turn.cartLost = true;
  if (strings(v.cartMissed) && v.cartMissed.length > 0) turn.cartMissed = v.cartMissed;
  return turn;
}

const CHAT_ID = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

// getRandomValues rather than randomUUID, which a page served over plain http doesn't have
export function newChatId(): string {
  const b = crypto.getRandomValues(new Uint8Array(16));
  b[6] = (b[6] & 0x0f) | 0x40;
  b[8] = (b[8] & 0x3f) | 0x80;
  const h = Array.from(b, (x) => x.toString(16).padStart(2, "0")).join("");
  return `${h.slice(0, 8)}-${h.slice(8, 12)}-${h.slice(12, 16)}-${h.slice(16, 20)}-${h.slice(20)}`;
}

function parse(raw: string | null): unknown {
  try {
    return raw ? JSON.parse(raw) : null;
  } catch {
    return null;
  }
}

export function readChatId(raw: string | null): string {
  const saved = parse(raw);
  return record(saved) && str(saved.id) && CHAT_ID.test(saved.id) ? saved.id : newChatId();
}

export function readTurns(raw: string | null): Turn[] {
  const saved = parse(raw);
  if (!record(saved) || !Array.isArray(saved.turns)) return [];
  return saved.turns.map(readTurn).filter((t): t is Turn => t !== null);
}

export function toMessages(turns: Turn[]): AssistantMessage[] {
  return turns.map(({ role, content, recap }) => (recap ? { role, content, recap } : { role, content }));
}

// A turn the shopper had somewhere else, carried into the chat as if it had happened there
export type Exchange = { question: string; reply: string; recap: string; products: ProductSummary[] };

export function withExchange(turns: Turn[], { question, reply, recap, products }: Exchange): Turn[] {
  const [asked, answered] = turns.slice(-2);
  if (asked?.role === "user" && asked.content === question && answered?.role === "assistant" && answered.content === reply) return turns;
  const turn: Turn = { role: "assistant", content: reply };
  if (recap) turn.recap = recap;
  if (products.length > 0) turn.products = products;
  return [...turns, { role: "user", content: question }, turn];
}

export function showedSomething(turn: Turn): boolean {
  return Boolean(turn.detail || turn.links?.length || turn.navigated || turn.products?.length || turn.showCart);
}
