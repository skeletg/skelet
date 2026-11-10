import { getCurrentChannelLocales } from "../locale/read";
import { boundedText } from "../revalidate-payload";
import { askAssistant, assistantPage, keepAssistantCart } from "./ask";
import type { AssistantStreamLine } from "./types";

const MAX_BODY_BYTES = 256 * 1024;

const json = (body: unknown, status = 200) => Response.json(body, { status, headers: { "Cache-Control": "no-store" } });

// The turn works on the shopper's cart under their cookies, so only the store's own pages may ask
export function sameOrigin(req: Request): boolean {
  const origin = req.headers.get("origin");
  if (!origin) {
    const site = req.headers.get("sec-fetch-site");
    return site === null || site === "same-origin";
  }
  const host = (req.headers.get("x-forwarded-host") ?? req.headers.get("host") ?? "").split(",")[0].trim();
  try {
    return host !== "" && new URL(origin).host === host;
  } catch {
    return false;
  }
}

async function jsonBody(req: Request): Promise<{ body: Record<string, unknown> | null } | Response> {
  if (!sameOrigin(req) || !(req.headers.get("content-type") ?? "").startsWith("application/json")) return json({ ok: false, problem: "failed" }, 403);
  const text = await boundedText(req, MAX_BODY_BYTES);
  if (text === null) return json({ ok: false, problem: "invalid" }, 413);
  try {
    return { body: JSON.parse(text) };
  } catch {
    return json({ ok: false, problem: "invalid" }, 400);
  }
}

async function shopperLocale(raw: unknown): Promise<string | undefined> {
  if (typeof raw !== "string" || raw === "") return undefined;
  const { codes } = await getCurrentChannelLocales();
  return codes.includes(raw) ? raw : undefined;
}

export async function postAssistant(req: Request): Promise<Response> {
  const parsed = await jsonBody(req);
  if (parsed instanceof Response) return parsed;
  const { messages, page, conversation, locale } = parsed.body ?? {};
  const shopper = await shopperLocale(locale);
  const encoder = new TextEncoder();
  const stop = new AbortController();
  req.signal.addEventListener("abort", () => stop.abort(), { once: true });
  if (req.signal.aborted) stop.abort();
  const stream = new ReadableStream<Uint8Array>({
    async start(controller) {
      const send = (line: AssistantStreamLine) => {
        if (!stop.signal.aborted) controller.enqueue(encoder.encode(JSON.stringify(line) + "\n"));
      };
      const answer = await askAssistant({
        messages,
        page,
        conversation,
        locale: shopper,
        signal: stop.signal,
        onStep: (step) => send({ step }),
        onCart: (id) => send({ cart: { id } }),
      });
      send({ answer });
      try {
        controller.close();
      } catch {
        // the page cancelled the stream already
      }
    },
    cancel() {
      stop.abort();
    },
  });
  return new Response(stream, { headers: { "Content-Type": "application/x-ndjson; charset=utf-8", "Cache-Control": "no-store", "X-Accel-Buffering": "no" } });
}

export async function putAssistantCart(req: Request): Promise<Response> {
  const parsed = await jsonBody(req);
  if (parsed instanceof Response) return parsed;
  const kept = await keepAssistantCart({ cartId: parsed.body?.cartId, final: parsed.body?.final, locale: await shopperLocale(parsed.body?.locale) });
  return json(kept, kept.ok ? 200 : 400);
}

export async function getAssistantPage(req: Request): Promise<Response> {
  const params = new URL(req.url).searchParams;
  return json(await assistantPage({ page: params.get("page") ?? "", locale: await shopperLocale(params.get("locale")) }));
}
