import { afterEach, describe, expect, it, vi } from "vitest";

import { readAssistantPage, sendToAssistant } from "./client";

const fetchMock = vi.fn();
vi.stubGlobal("fetch", fetchMock);
afterEach(() => fetchMock.mockReset());

describe("sendToAssistant", () => {
  it("posts the clipped conversation as JSON and returns the answer", async () => {
    fetchMock.mockResolvedValue(Response.json({ ok: true, reply: "Hi" }));
    const answer = await sendToAssistant([{ role: "assistant", content: "y".repeat(2100) }, { role: "user", content: "hi" }], "/mugs");
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/assistant");
    expect(init.headers).toEqual({ "Content-Type": "application/json" });
    const body = JSON.parse(init.body);
    expect(body.page).toBe("/mugs");
    expect(body.messages[0].content).toHaveLength(2000);
    expect(answer).toEqual({ ok: true, reply: "Hi" });
  });

  it("answers failed for a broken response, and rethrows an abort", async () => {
    fetchMock.mockResolvedValue(new Response("<html>", { status: 502 }));
    expect(await sendToAssistant([{ role: "user", content: "hi" }], "/")).toEqual({ ok: false, problem: "failed" });
    const ctrl = new AbortController();
    fetchMock.mockImplementation(() => {
      ctrl.abort();
      return Promise.reject(new DOMException("aborted", "AbortError"));
    });
    await expect(sendToAssistant([{ role: "user", content: "hi" }], "/", { signal: ctrl.signal })).rejects.toBeDefined();
  });
});

describe("sendToAssistant over the stream", () => {
  const streamed = (lines: unknown[]) => new Response(lines.map((l) => JSON.stringify(l)).join("\n") + "\n", { headers: { "Content-Type": "application/x-ndjson" } });

  it("hears each step, then hands the store a guest's new cart before answering", async () => {
    fetchMock.mockResolvedValueOnce(streamed([{ step: { tool: "search_products", detail: "mugs" } }, { step: { tool: "update_cart", detail: "" } }, { answer: { ok: true, reply: "Done", keepCart: "c1" } }]));
    fetchMock.mockResolvedValueOnce(Response.json({ ok: true }));
    const steps: unknown[] = [];
    const answer = await sendToAssistant([{ role: "user", content: "two mugs" }], "/", { onStep: (s) => steps.push(s), conversation: "chat-1" });
    expect(JSON.parse(fetchMock.mock.calls[0][1].body).conversation).toBe("chat-1");
    expect(steps).toEqual([
      { tool: "search_products", detail: "mugs" },
      { tool: "update_cart", detail: "" },
    ]);
    expect(answer).toEqual({ ok: true, reply: "Done" });
    const [url, init] = fetchMock.mock.calls[1];
    expect(url).toBe("/api/assistant");
    expect(init.method).toBe("PUT");
    expect(JSON.parse(init.body)).toEqual({ cartId: "c1", final: true });
  });

  it("keeps a cart the moment the turn makes it, so leaving early still leaves it with the shopper", async () => {
    const kept = { id: "c1", lines: [] };
    fetchMock.mockResolvedValueOnce(streamed([{ cart: { id: "c1" } }, { answer: { ok: true, reply: "Done", keepCart: "c1", cart: kept } }]));
    fetchMock.mockResolvedValueOnce(Response.json({ ok: true, cart: kept }));
    const carts: unknown[] = [];
    const ctrl = new AbortController();
    const answer = await sendToAssistant([{ role: "user", content: "two mugs" }], "/", { onCart: (c) => carts.push(c), signal: ctrl.signal });
    expect(carts).toEqual([kept]);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(JSON.parse(fetchMock.mock.calls[1][1].body)).toEqual({ cartId: "c1", final: false });
    expect(fetchMock.mock.calls[1][1].signal).toBeUndefined();
    expect(answer).toEqual({ ok: true, reply: "Done", cart: kept });
  });

  it("shows the shopper's own cart with the turn's lines when they made one meanwhile", async () => {
    const theirs = { id: "own", lines: [{ id: "l1" }, { id: "l2" }] };
    fetchMock.mockResolvedValueOnce(streamed([{ cart: { id: "c1" } }, { answer: { ok: true, reply: "Done", keepCart: "c1", cart: { id: "c1" } } }]));
    fetchMock.mockResolvedValueOnce(Response.json({ ok: true, pending: true }));
    fetchMock.mockResolvedValueOnce(Response.json({ ok: true, cart: theirs }));
    const carts: unknown[] = [];
    const answer = await sendToAssistant([{ role: "user", content: "two mugs" }], "/", { onCart: (c) => carts.push(c) });
    expect(carts).toEqual([]);
    expect(JSON.parse(fetchMock.mock.calls[2][1].body)).toEqual({ cartId: "c1", final: true });
    expect(answer).toEqual({ ok: true, reply: "Done", cart: theirs });
  });

  it("tries a dropped keep once more, a final one too, since its lines leave the turn's cart as they go in", async () => {
    const kept = { id: "c1", lines: [] };
    fetchMock.mockResolvedValueOnce(streamed([{ cart: { id: "c1" } }, { answer: { ok: true, reply: "Done" } }]));
    fetchMock.mockRejectedValueOnce(new TypeError("network"));
    fetchMock.mockResolvedValueOnce(Response.json({ ok: true, cart: kept }));
    const carts: unknown[] = [];
    await sendToAssistant([{ role: "user", content: "two mugs" }], "/", { onCart: (c) => carts.push(c) });
    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(carts).toEqual([kept]);

    const theirs = { id: "own", lines: [{ id: "l1" }] };
    fetchMock.mockReset();
    fetchMock.mockResolvedValueOnce(streamed([{ answer: { ok: true, reply: "Done", keepCart: "c1", cart: { id: "c1" } } }]));
    fetchMock.mockResolvedValueOnce(new Response("down", { status: 503 }));
    fetchMock.mockResolvedValueOnce(Response.json({ ok: true, cart: theirs, missed: ["Enamel mug"] }));
    expect(await sendToAssistant([{ role: "user", content: "two mugs" }], "/")).toEqual({ ok: true, reply: "Done", cart: theirs, cartMissed: ["Enamel mug"] });
    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(JSON.parse(fetchMock.mock.calls[2][1].body)).toEqual({ cartId: "c1", final: true });
  });

  it("tells the shopper when the turn's cart never reached theirs", async () => {
    fetchMock.mockResolvedValueOnce(streamed([{ answer: { ok: true, reply: "Done", keepCart: "c1", cart: { id: "c1" } } }]));
    fetchMock.mockRejectedValue(new TypeError("network"));
    expect(await sendToAssistant([{ role: "user", content: "two mugs" }], "/")).toEqual({ ok: true, reply: "Done", cartLost: true });
    expect(fetchMock).toHaveBeenCalledTimes(3);

    fetchMock.mockReset();
    fetchMock.mockResolvedValueOnce(streamed([{ answer: { ok: true, reply: "Done", keepCart: "c1", cart: { id: "c1" } } }]));
    fetchMock.mockResolvedValueOnce(Response.json({ ok: false }, { status: 400 }));
    expect(await sendToAssistant([{ role: "user", content: "two mugs" }], "/")).toEqual({ ok: true, reply: "Done", cartLost: true });
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("still hands over the cart a keep in flight lands, then gives up the turn the page dropped", async () => {
    const kept = { id: "c1", lines: [] };
    const ctrl = new AbortController();
    const encoder = new TextEncoder();
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(encoder.encode(JSON.stringify({ cart: { id: "c1" } }) + "\n"));
        ctrl.signal.addEventListener("abort", () => controller.error(ctrl.signal.reason));
      },
    });
    let land: (r: Response) => void = () => {};
    fetchMock.mockResolvedValueOnce(new Response(body, { headers: { "Content-Type": "application/x-ndjson" } }));
    fetchMock.mockReturnValueOnce(new Promise<Response>((done) => (land = done)));
    const carts: unknown[] = [];
    const turn = sendToAssistant([{ role: "user", content: "two mugs" }], "/", { signal: ctrl.signal, onCart: (c) => carts.push(c) });
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    ctrl.abort(new DOMException("left", "AbortError"));
    land(Response.json({ ok: true, cart: kept }));
    await expect(turn).rejects.toMatchObject({ name: "AbortError" });
    expect(carts).toEqual([kept]);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("sends the shopper's language with the turn and every keep", async () => {
    fetchMock.mockResolvedValueOnce(streamed([{ cart: { id: "c1" } }, { answer: { ok: true, reply: "Fertig", keepCart: "c2" } }]));
    fetchMock.mockResolvedValue(Response.json({ ok: true }));
    await sendToAssistant([{ role: "user", content: "zwei Tassen" }], "/de", { locale: "de" });
    expect(fetchMock.mock.calls.map(([, init]) => JSON.parse(init.body).locale)).toEqual(["de", "de", "de"]);
  });

  it("answers failed when the stream ends without an answer", async () => {
    fetchMock.mockResolvedValueOnce(streamed([{ step: { tool: "get_cart", detail: "" } }]));
    expect(await sendToAssistant([{ role: "user", content: "hi" }], "/")).toEqual({ ok: false, problem: "failed" });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});

describe("readAssistantPage", () => {
  it("asks for the page by its path, and gives way quietly", async () => {
    fetchMock.mockResolvedValue(Response.json({ kind: "listing", title: "Mugs" }));
    expect(await readAssistantPage("/mugs?x=1")).toEqual({ kind: "listing", title: "Mugs" });
    expect(fetchMock.mock.calls[0][0]).toBe("/api/assistant?page=%2Fmugs%3Fx%3D1");
    await readAssistantPage("/de/mugs", { locale: "de" });
    expect(fetchMock.mock.calls[1][0]).toBe("/api/assistant?page=%2Fde%2Fmugs&locale=de");
    fetchMock.mockResolvedValue(Response.json(null));
    expect(await readAssistantPage("/")).toBeNull();
  });
});
