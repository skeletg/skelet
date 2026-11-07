import type { Cart } from "../cart/types";
import { ndjson } from "./ndjson";
import { cleanMessages, cleanStep, type AssistantAnswer, type AssistantMessage, type AssistantPage, type AssistantStep, type KeptCart, type StreamedAnswer } from "./types";

export const ASSISTANT_ROUTE = "/api/assistant";

type SendOptions = { signal?: AbortSignal; route?: string; onStep?: (step: AssistantStep) => void; onCart?: (cart: Cart | null) => void; conversation?: string; locale?: string };

export async function sendToAssistant(messages: AssistantMessage[], page: string, { signal, route = ASSISTANT_ROUTE, onStep, onCart, conversation, locale }: SendOptions = {}): Promise<AssistantAnswer> {
  let kept: string | null = null;
  try {
    const res = await fetch(route, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ messages: cleanMessages(messages), page, conversation, locale }),
      cache: "no-store",
      signal,
    });
    if (!(res.headers.get("content-type") ?? "").includes("ndjson") || !res.body) return answerOf(await res.json()) ?? FAILED;
    for await (const line of ndjson(res.body)) {
      const step = cleanStep(line.step);
      if (step) onStep?.(step);
      const made = cartIdOf(line.cart);
      if (made) {
        const result = await keep(route, made, false, locale);
        if (result?.ok && !result.pending) {
          kept = made;
          if (result.cart !== undefined) onCart?.(result.cart);
        }
        continue;
      }
      const answer = answerOf(line.answer);
      if (!answer) continue;
      const { keepCart, ...rest } = answer as StreamedAnswer;
      if (typeof keepCart === "string" && keepCart !== kept && rest.ok) {
        const result = await keep(route, keepCart, true, locale);
        if (!result?.ok) {
          delete rest.cart;
          rest.cartLost = true;
        } else {
          if (result.cart !== undefined) rest.cart = result.cart;
          if (Array.isArray(result.missed) && result.missed.length > 0) rest.cartMissed = result.missed.filter((n) => typeof n === "string");
        }
      }
      return rest;
    }
  } catch (err) {
    if (signal?.aborted) throw signal.reason;
    void err;
  }
  return FAILED;
}

const FAILED: AssistantAnswer = { ok: false, problem: "failed" };

function answerOf(body: unknown): AssistantAnswer | null {
  return body && typeof body === "object" && typeof (body as { ok?: unknown }).ok === "boolean" ? (body as AssistantAnswer) : null;
}

function cartIdOf(raw: unknown): string | null {
  const id = raw && typeof raw === "object" ? (raw as { id?: unknown }).id : null;
  return typeof id === "string" && id !== "" ? id : null;
}

// Not tied to the turn's signal, and safe to send twice - a final keep moves each line out of the turn's cart as it adds it
async function keep(route: string, cartId: string, final: boolean, locale: string | undefined): Promise<KeptCart | null> {
  for (let attempt = 0; attempt < 2; attempt++) {
    const res = await fetch(route, { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ cartId, final, locale }), cache: "no-store" }).catch(() => null);
    if (res && res.status < 500) {
      const body: unknown = await res.json().catch(() => null);
      return body && typeof body === "object" && typeof (body as { ok?: unknown }).ok === "boolean" ? (body as KeptCart) : { ok: false };
    }
  }
  return null;
}

export async function readAssistantPage(page: string, { signal, locale, route = ASSISTANT_ROUTE }: { signal?: AbortSignal; locale?: string; route?: string } = {}): Promise<AssistantPage | null> {
  try {
    const res = await fetch(`${route}?${new URLSearchParams(locale ? { page, locale } : { page })}`, { cache: "no-store", signal });
    const body: unknown = await res.json();
    return body && typeof body === "object" && typeof (body as AssistantPage).kind === "string" ? (body as AssistantPage) : null;
  } catch {
    return null;
  }
}
