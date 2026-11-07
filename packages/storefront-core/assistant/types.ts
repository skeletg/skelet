import type { Cart } from "../cart/types";
import type { ProductSummary } from "../catalog/read";

export const MAX_TURNS = 30;
export const MAX_MESSAGE_CHARS = 2000;
export const MAX_RECAP_CHARS = 600;
export const MAX_STEP_DETAIL = 80;

export type AssistantMessage = { role: "user" | "assistant"; content: string; recap?: string };

export type AssistantProblem = "unavailable" | "busy" | "limited" | "invalid" | "failed";

export type AssistantSpec = { label: string; value: string | boolean };
export type AssistantNotes = { summary: string; highlights: { title: string; detail: string }[]; goodFor: string[]; keepInMind: string[] };
export type AssistantDetail = { product: ProductSummary; specs: AssistantSpec[]; notes: AssistantNotes };
// count is how many products a listing shows, null for a page
export type AssistantLink = { label: string; path: string; query: string; filters: { slug: string; values: string[] }[]; sort: string; count: number | null };

export type AssistantAnswer =
  | {
      ok: true;
      reply: string;
      incomplete: boolean;
      recap: string;
      followUps: string[];
      products: ProductSummary[];
      detail: AssistantDetail | null;
      links: AssistantLink[];
      navigate: AssistantLink | null;
      cartChanged: boolean;
      cart?: Cart | null;
      // what the turn put in its own cart didn't reach the shopper's - all of it, or the lines named
      cartLost?: boolean;
      cartMissed?: string[];
      checkout: boolean;
    }
  | { ok: false; problem: AssistantProblem };

export type AssistantStep = { tool: string; detail: string };

export type StreamedAnswer = AssistantAnswer & { keepCart?: string };

export type AssistantStreamLine = { step: AssistantStep } | { cart: { id: string } } | { answer: StreamedAnswer };

export type KeptCart = { ok: boolean; cart?: Cart | null; pending?: boolean; missed?: string[] };

export function cleanStep(raw: unknown): AssistantStep | null {
  if (!raw || typeof raw !== "object") return null;
  const { tool, detail } = raw as { tool?: unknown; detail?: unknown };
  return typeof tool === "string" && tool !== "" ? { tool, detail: typeof detail === "string" ? clip(detail, MAX_STEP_DETAIL) : "" } : null;
}

export type AssistantPage = { kind: string; title: string };

function clip(text: string, max: number): string {
  return text.length > max ? text.slice(0, max) : text;
}

export function cleanMessages(raw: unknown): AssistantMessage[] {
  if (!Array.isArray(raw)) return [];
  const out = raw.flatMap((m): AssistantMessage[] => {
    if (!m || (m.role !== "user" && m.role !== "assistant") || typeof m.content !== "string") return [];
    const content = clip(m.content, MAX_MESSAGE_CHARS);
    return m.role === "assistant" && typeof m.recap === "string" && m.recap !== ""
      ? [{ role: m.role, content, recap: clip(m.recap, MAX_RECAP_CHARS) }]
      : [{ role: m.role, content }];
  });
  return out.slice(-MAX_TURNS);
}
