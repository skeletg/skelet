import type { TypedDocumentNode } from "@graphql-typed-document-node/core";
import { parse } from "graphql";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

const CHANNEL = "56d051bc-0131-4476-bc0b-121d80290684";
let gqlMod: typeof import("./gql");

beforeAll(async () => {
  process.env.BACKEND_INTERNAL_URL = "http://backend.test";
  gqlMod = await import("./gql");
  // The request the core's client reads the channel from - Astro's middleware answers it in the app
  (await import("@skelet/storefront-core/testing")).useTestRequest({ channelId: async () => CHANNEL });
});

afterEach(() => vi.unstubAllGlobals());

type Ping = { ping: boolean };
const doc = parse("query Ping { ping }") as TypedDocumentNode<Ping, Record<string, never>>;
const withVars = doc as unknown as TypedDocumentNode<Ping, { a: number }>;

function stubFetch(res: Response | Error) {
  const fetch = vi.fn(async () => {
    if (res instanceof Error) throw res;
    return res;
  });
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

describe("gql", () => {
  it("sends the pinned channel, the operation name and the variables", async () => {
    const fetch = stubFetch(Response.json({ data: { ping: true } }));
    const data = await gqlMod.gql(withVars, { a: 1 });
    expect(data).toEqual({ ping: true });
    const [url, init] = fetch.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe("http://backend.test/api/front/graphql");
    expect((init.headers as Record<string, string>)["X-Channel-Id"]).toBe(CHANNEL);
    expect(JSON.parse(String(init.body))).toMatchObject({ operationName: "Ping", variables: { a: 1 } });
  });

  it("surfaces the backend's error code", async () => {
    stubFetch(Response.json({ errors: [{ message: "only 2 left", extensions: { code: "CART_INSUFFICIENT_STOCK", class: "CONFLICT" } }] }));
    await expect(gqlMod.gql(doc)).rejects.toMatchObject({ message: "only 2 left", code: "CART_INSUFFICIENT_STOCK" });
  });

  it("reports a non-200 or an unreachable backend as UNAVAILABLE", async () => {
    stubFetch(new Response("nope", { status: 502 }));
    await expect(gqlMod.gql(doc)).rejects.toMatchObject({ code: "UNAVAILABLE" });
    stubFetch(new TypeError("ECONNREFUSED"));
    await expect(gqlMod.gql(doc)).rejects.toMatchObject({ code: "UNAVAILABLE" });
  });
});

describe("shopperMessage", () => {
  it("passes a resolver's own message through", () => {
    expect(gqlMod.shopperMessage(new gqlMod.GqlError("only 2 left", "CART_INSUFFICIENT_STOCK"), "fallback")).toBe("only 2 left");
  });

  it("hides outages and internal errors behind the fallback", () => {
    const spy = vi.spyOn(console, "error").mockImplementation(() => {});
    expect(gqlMod.shopperMessage(new gqlMod.GqlError("backend HTTP 502", "UNAVAILABLE"), "fallback")).toBe("fallback");
    expect(gqlMod.shopperMessage(new Error("boom"), "fallback")).toBe("fallback");
    spy.mockRestore();
  });
});
