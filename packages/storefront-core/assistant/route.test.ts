import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ askAssistant: vi.fn(), assistantPage: vi.fn(), keepAssistantCart: vi.fn() }));
vi.mock("./ask", () => mocks);
vi.mock("../locale/read", () => ({ getCurrentChannelLocales: async () => ({ codes: ["en", "de"], defaultLocale: "en", hreflang: {} }) }));

const { getAssistantPage, postAssistant, putAssistantCart, sameOrigin } = await import("./route");

const post = (body: string, headers: Record<string, string> = {}, method = "POST") =>
  new Request("https://shop.example/api/assistant", {
    method,
    body,
    headers: { host: "shop.example", origin: "https://shop.example", "content-type": "application/json", ...headers },
  });

beforeEach(() => vi.clearAllMocks());

describe("postAssistant", () => {
  it("streams each step as it starts, then the answer", async () => {
    mocks.askAssistant.mockImplementation(async ({ onStep, onCart }: { onStep: (s: unknown) => void; onCart: (id: string) => void }) => {
      onStep({ tool: "search_products", detail: "mugs" });
      onCart("c1");
      onStep({ tool: "get_product", detail: "Big mug" });
      return { ok: true, reply: "Hi", keepCart: "c1" };
    });
    const res = await postAssistant(post(JSON.stringify({ messages: [{ role: "user", content: "hi" }], page: "/mugs", conversation: "chat-1" })));
    expect(mocks.askAssistant).toHaveBeenCalledWith(expect.objectContaining({ messages: [{ role: "user", content: "hi" }], page: "/mugs", conversation: "chat-1", signal: expect.any(AbortSignal) }));
    expect(res.headers.get("cache-control")).toBe("no-store");
    expect(res.headers.get("content-type")).toContain("application/x-ndjson");
    expect((await res.text()).trim().split("\n").map((l) => JSON.parse(l))).toEqual([
      { step: { tool: "search_products", detail: "mugs" } },
      { cart: { id: "c1" } },
      { step: { tool: "get_product", detail: "Big mug" } },
      { answer: { ok: true, reply: "Hi", keepCart: "c1" } },
    ]);
  });

  it("asks in the shopper's language when the store speaks it", async () => {
    mocks.askAssistant.mockResolvedValue({ ok: false, problem: "failed" });
    await (await postAssistant(post(JSON.stringify({ messages: [], locale: "de" })))).text();
    expect(mocks.askAssistant).toHaveBeenLastCalledWith(expect.objectContaining({ locale: "de" }));
    for (const locale of ["fr", 7, ""]) {
      await (await postAssistant(post(JSON.stringify({ messages: [], locale })))).text();
      expect(mocks.askAssistant.mock.lastCall?.[0].locale).toBeUndefined();
    }
  });

  it("stops a turn whose page had already gone before it started", async () => {
    let seen: AbortSignal | undefined;
    mocks.askAssistant.mockImplementation(async ({ signal }: { signal: AbortSignal }) => {
      seen = signal;
      return { ok: false, problem: "failed" };
    });
    const ctrl = new AbortController();
    ctrl.abort();
    const req = new Request(post(JSON.stringify({ messages: [] })), { signal: ctrl.signal });
    const res = await postAssistant(req);
    expect(await res.text()).toBe("");
    expect(seen?.aborted).toBe(true);
  });

  it("stops the turn when the page goes away", async () => {
    let seen: AbortSignal | undefined;
    mocks.askAssistant.mockImplementation(async ({ signal }: { signal: AbortSignal }) => {
      seen = signal;
      await new Promise((resolve) => signal.addEventListener("abort", resolve));
      return { ok: false, problem: "failed" };
    });
    const res = await postAssistant(post(JSON.stringify({ messages: [] })));
    await res.body!.cancel();
    expect(seen?.aborted).toBe(true);
  });

  it("refuses another site's page, and a body that isn't JSON", async () => {
    expect((await postAssistant(post("{}", { origin: "https://evil.example" }))).status).toBe(403);
    expect((await postAssistant(post("{}", { "content-type": "text/plain" }))).status).toBe(403);
    expect((await postAssistant(post("{not json"))).status).toBe(400);
    expect((await postAssistant(post("x".repeat(300 * 1024)))).status).toBe(413);
    expect(mocks.askAssistant).not.toHaveBeenCalled();
  });
});

describe("putAssistantCart", () => {
  it("keeps the cart a turn made, for the store's own pages only", async () => {
    mocks.keepAssistantCart.mockResolvedValue({ ok: true, cart: { id: "c1" } });
    const res = await putAssistantCart(post(JSON.stringify({ cartId: "c1", final: true }), {}, "PUT"));
    expect(mocks.keepAssistantCart).toHaveBeenCalledWith({ cartId: "c1", final: true, locale: undefined });
    expect(await res.json()).toEqual({ ok: true, cart: { id: "c1" } });
    mocks.keepAssistantCart.mockResolvedValue({ ok: false });
    expect((await putAssistantCart(post(JSON.stringify({ cartId: "nope" }), {}, "PUT"))).status).toBe(400);
    mocks.keepAssistantCart.mockClear();
    expect((await putAssistantCart(post(JSON.stringify({ cartId: "c1" }), { origin: "https://evil.example" }, "PUT"))).status).toBe(403);
    expect(mocks.keepAssistantCart).not.toHaveBeenCalled();
    await putAssistantCart(post(JSON.stringify({ cartId: "c1", final: true, locale: "de" }), {}, "PUT"));
    expect(mocks.keepAssistantCart).toHaveBeenCalledWith({ cartId: "c1", final: true, locale: "de" });
  });
});

describe("sameOrigin", () => {
  const req = (headers: Record<string, string>) => new Request("https://shop.example/api/assistant", { headers });
  it("trusts the store's own pages and the host a proxy names", () => {
    expect(sameOrigin(req({ host: "shop.example", origin: "https://shop.example" }))).toBe(true);
    expect(sameOrigin(req({ host: "app:3000", "x-forwarded-host": "shop.example", origin: "https://shop.example" }))).toBe(true);
    expect(sameOrigin(req({ "sec-fetch-site": "same-origin" }))).toBe(true);
    expect(sameOrigin(req({ "sec-fetch-site": "cross-site" }))).toBe(false);
    expect(sameOrigin(req({ host: "shop.example", origin: "null" }))).toBe(false);
  });
});

describe("getAssistantPage", () => {
  it("reads the page named in the query", async () => {
    mocks.assistantPage.mockResolvedValue({ kind: "product", title: "Mug" });
    const res = await getAssistantPage(new Request("https://shop.example/api/assistant?page=%2Fshop%2Fmug%3Fx%3D1"));
    expect(mocks.assistantPage).toHaveBeenCalledWith({ page: "/shop/mug?x=1", locale: undefined });
    expect(await res.json()).toEqual({ kind: "product", title: "Mug" });
    await getAssistantPage(new Request("https://shop.example/api/assistant?page=%2Fde%2Fshop%2Fmug&locale=de"));
    expect(mocks.assistantPage).toHaveBeenLastCalledWith({ page: "/de/shop/mug", locale: "de" });
    await getAssistantPage(new Request("https://shop.example/api/assistant?page=%2Fshop%2Fmug&locale=xx"));
    expect(mocks.assistantPage).toHaveBeenLastCalledWith({ page: "/shop/mug", locale: undefined });
  });
});
