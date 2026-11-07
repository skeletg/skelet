import { backendHeaders, gqlFetch } from "../graphql";
import { FRONT_ASSISTANT_URL } from "../backend-url";
import type { AssistantReply } from "../graphql/schema.types";
import { request, rethrowControlFlow } from "../request";
import { getAccessToken } from "../auth/session";
import { setCartCookie } from "../cart/actions";
import { CART_COOKIE } from "../cart/read";
import { CartByIdDocument, MyCartDocument } from "../cart/queries.gql";
import { AddToCartDocument, ApplyCouponDocument, RemoveFromCartDocument } from "../cart/mutations.gql";
import { getResolvedConsent } from "../consent/read";
import type { Cart } from "../cart/types";
import { CompareProductsDocument, ProductsByIdsDocument, type CompareProductsQuery } from "../catalog/operations.gql";
import type { ProductSummary } from "../catalog/read";
import { AssistantPageDocument } from "./assistant.gql";
import { ndjson } from "./ndjson";
import { isStorePath, pageForAssistant } from "./page";
import { cleanMessages, cleanStep, type AssistantDetail, type AssistantLink, type AssistantNotes, type AssistantPage, type AssistantProblem, type AssistantSpec, type AssistantStep, type KeptCart, type StreamedAnswer } from "./types";

export type * from "./types";

const MAX_SPECS = 8;

function plainLink(l: Omit<AssistantLink, "count"> & { count?: number | null }): AssistantLink {
  return { label: l.label, path: l.path, query: l.query, filters: l.filters.map((f) => ({ slug: f.slug, values: [...f.values] })), sort: l.sort, count: l.count ?? null };
}

const PROBLEMS: Record<string, AssistantProblem> = {
  ASSISTANT_UNAVAILABLE: "unavailable",
  ASSISTANT_BUSY: "busy",
  ASSISTANT_RATE_LIMITED: "limited",
  RATE_LIMITED: "limited",
  ASSISTANT_MESSAGE_INVALID: "invalid",
};

class Refused extends Error {
  constructor(readonly code: string) {
    super(code);
  }
}

type Call = { locale?: string; signal?: AbortSignal };

async function streamTurn(body: unknown, { locale, signal }: Call, onStep: (step: AssistantStep) => void, onCart: (id: string) => void): Promise<AssistantReply> {
  let res: Response;
  try {
    res = await request().fetch(FRONT_ASSISTANT_URL, { method: "POST", headers: await backendHeaders({ locale }), body: JSON.stringify(body), signal });
  } catch (err) {
    rethrowControlFlow(err);
    if (signal?.aborted) throw err;
    console.error(`assistant: calling ${FRONT_ASSISTANT_URL} failed:`, err);
    throw new Refused("FAILED");
  }
  if (!res.ok || !res.body) throw new Refused(res.status === 429 ? "RATE_LIMITED" : "FAILED");
  for await (const line of ndjson(res.body)) {
    const step = cleanStep(line.step);
    const cart = line.cart && typeof line.cart === "object" ? (line.cart as { id?: unknown }).id : undefined;
    if (step) onStep(step);
    else if (isUUID(cart)) onCart(cart);
    else if (line.reply && typeof line.reply === "object") return line.reply as AssistantReply;
    else if (line.error && typeof line.error === "object") throw new Refused(String((line.error as { code?: unknown }).code));
  }
  throw new Refused("FAILED");
}

// The chat id is what a store that keeps transcripts files the turn under, so it goes only with the shopper's analytics consent
export async function askAssistant(input: {
  messages: unknown;
  page?: unknown;
  conversation?: unknown;
  locale?: string;
  signal?: AbortSignal;
  onStep?: (step: AssistantStep) => void;
  onCart?: (cartId: string) => void;
}): Promise<StreamedAnswer> {
  const messages = cleanMessages(input?.messages);
  if (messages.length === 0 || messages[messages.length - 1].role !== "user") return { ok: false, problem: "invalid" };
  try {
    const signedIn = Boolean(await getAccessToken());
    const jar = await request().cookies();
    const guestCart = signedIn ? null : (jar.get(CART_COOKIE)?.value ?? null);
    const keepsChat = isUUID(input.conversation) && (await getResolvedConsent()).analytics;
    const call: Call = { locale: input.locale, signal: input.signal };
    const a = await streamTurn(
      { messages, cartId: guestCart, page: cleanPage(input.page), conversationId: keepsChat ? input.conversation : null },
      call,
      input.onStep ?? (() => {}),
      (id) => {
        if (!signedIn && id !== guestCart) input.onCart?.(id);
      },
    );
    const [products, detail, cart] = await Promise.all([
      cards(a.productIds, call),
      details(a.detail, call),
      a.cartChanged ? freshCart(signedIn, a.cartId ?? guestCart, call) : undefined,
    ]);
    const answer: StreamedAnswer = {
      ok: true,
      reply: a.reply,
      incomplete: a.incomplete,
      recap: a.recap,
      followUps: [...a.followUps],
      products,
      detail,
      links: a.links.filter((l) => isStorePath(l.path)).map(plainLink),
      navigate: a.navigate && isStorePath(a.navigate.path) ? plainLink(a.navigate) : null,
      cartChanged: a.cartChanged,
      checkout: a.checkout,
    };
    if (cart !== undefined) answer.cart = cart;
    if (!signedIn && a.cartId && a.cartId !== guestCart) answer.keepCart = a.cartId;
    return answer;
  } catch (err) {
    rethrowControlFlow(err);
    return { ok: false, problem: (err instanceof Refused && PROBLEMS[err.code]) || "failed" };
  }
}

const isUUID = (v: unknown): v is string => typeof v === "string" && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(v);

function cleanPage(raw: unknown): string | null {
  if (typeof raw !== "string" || !isStorePath(raw)) return null;
  const url = new URL(raw, "http://store.invalid");
  return pageForAssistant(url.pathname, url.search) || null;
}

// A guest who made a cart of their own while the turn ran keeps theirs, and the turn's lines join it once the turn is done
export async function keepAssistantCart(input: { cartId: unknown; final?: unknown; locale?: string }): Promise<KeptCart> {
  const id = input?.cartId;
  if (!isUUID(id)) return { ok: false };
  try {
    if (await getAccessToken()) return { ok: false };
    const call = { locale: input.locale };
    const made = (await gqlFetch(CartByIdDocument, { id }, call)).data.cart;
    if (!made) return { ok: false };
    const jar = await request().cookies();
    const current = jar.get(CART_COOKIE)?.value;
    const own = current && current !== id ? (await gqlFetch(CartByIdDocument, { id: current }, call)).data.cart : null;
    if (!own) {
      setCartCookie(jar, id);
      return { ok: true, cart: made };
    }
    if (input.final !== true) return { ok: true, pending: true };
    // Each line leaves the turn's cart as it joins theirs, so a merge sent again never adds one twice
    let cart = own;
    const missed: string[] = [];
    for (const line of made.lines) {
      try {
        if (!line.sku) throw new Error("line has no sku");
        cart = (await gqlFetch(AddToCartDocument, { cartId: own.id, sku: line.sku, quantity: line.quantity }, call)).data.addToCart;
      } catch (err) {
        rethrowControlFlow(err);
        missed.push(line.name);
        continue;
      }
      try {
        await gqlFetch(RemoveFromCartDocument, { cartId: made.id, lineId: line.id }, call);
      } catch (err) {
        rethrowControlFlow(err);
      }
    }
    for (const { code } of made.appliedCodes) {
      try {
        cart = (await gqlFetch(ApplyCouponDocument, { cartId: own.id, code }, call)).data.applyCoupon;
      } catch (err) {
        rethrowControlFlow(err);
      }
    }
    return missed.length > 0 ? { ok: true, cart, missed } : { ok: true, cart };
  } catch (err) {
    rethrowControlFlow(err);
    return { ok: false };
  }
}

async function cards(ids: string[], call: Call): Promise<ProductSummary[]> {
  if (ids.length === 0) return [];
  try {
    return (await gqlFetch(ProductsByIdsDocument, { ids }, call)).data.productsByIds as unknown as ProductSummary[];
  } catch (err) {
    rethrowControlFlow(err);
    return [];
  }
}

type AttributeValue = CompareProductsQuery["productsByIds"][number]["attributeValues"][number];

function specValue(av: AttributeValue): string | boolean | null {
  if (av.selectedValues.length > 0) return av.selectedValues.map((v) => v.name).join(", ");
  const labels = av.references.map((r) => r.label).filter(Boolean);
  if (labels.length > 0) return labels.join(", ");
  if (typeof av.scalarValue === "boolean") return av.scalarValue;
  if (av.scalarValue == null || av.scalarValue === "") return null;
  return `${String(av.scalarValue)}${av.attribute.unit ? ` ${av.attribute.unit}` : ""}`;
}

// The same public attributes the compare page shows, so the panel never says more than the store does
type ReplyDetail = { productId: string; summary: string; highlights: { title: string; detail: string }[]; goodFor: string[]; keepInMind: string[] };

async function details(detail: ReplyDetail | null | undefined, call: Call): Promise<AssistantDetail | null> {
  if (!detail) return null;
  const notes: AssistantNotes = {
    summary: detail.summary,
    highlights: detail.highlights.map((h) => ({ title: h.title, detail: h.detail })),
    goodFor: [...detail.goodFor],
    keepInMind: [...detail.keepInMind],
  };
  try {
    const found = (await gqlFetch(CompareProductsDocument, { ids: [detail.productId] }, call)).data.productsByIds[0];
    if (!found) return null;
    const { attributeValues, ...product } = found;
    const seen = new Set<string>();
    const specs: AssistantSpec[] = [];
    for (const av of attributeValues) {
      const value = av.attribute.isPublic && !seen.has(av.attribute.slug) ? specValue(av) : null;
      if (value === null || specs.length >= MAX_SPECS) continue;
      seen.add(av.attribute.slug);
      specs.push({ label: av.attribute.name, value });
    }
    return { product: product as unknown as ProductSummary, specs, notes };
  } catch (err) {
    rethrowControlFlow(err);
    return null;
  }
}

// undefined when it can't be read, so the store keeps what it shows rather than emptying the cart
async function freshCart(signedIn: boolean, id: string | null, call: Call): Promise<Cart | null | undefined> {
  try {
    if (signedIn) return (await gqlFetch(MyCartDocument, {}, call)).data.myCart;
    return id ? (await gqlFetch(CartByIdDocument, { id }, call)).data.cart : undefined;
  } catch (err) {
    rethrowControlFlow(err);
    return undefined;
  }
}

export async function assistantPage(input: { page: unknown; locale?: string }): Promise<AssistantPage | null> {
  const page = cleanPage(input?.page);
  if (!page) return null;
  try {
    const { assistantPage: p } = (await gqlFetch(AssistantPageDocument, { page }, { locale: input.locale })).data;
    return { kind: p.kind, title: p.title };
  } catch (err) {
    rethrowControlFlow(err);
    return null;
  }
}
