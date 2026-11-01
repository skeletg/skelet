import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { readRevalidate, warmURL } from "./revalidate-payload";

const CH = "11111111-2222-4333-8444-555555555555";
const post = (body: string, secret = "s3cret", headers: Record<string, string> = {}) =>
  new Request("http://shop.test/api/revalidate", { method: "POST", headers: { "x-revalidate-secret": secret, ...headers }, body });

beforeEach(() => vi.stubEnv("STOREFRONT_REVALIDATE_SECRET", "s3cret"));
afterEach(() => vi.unstubAllEnvs());

describe("readRevalidate", () => {
  it("refuses a wrong or missing secret before reading the body", async () => {
    expect(((await readRevalidate(post("{}", "nope"))) as Response).status).toBe(401);
    expect(((await readRevalidate(new Request("http://shop.test/", { method: "POST", body: "{}" }))) as Response).status).toBe(401);
    vi.stubEnv("STOREFRONT_REVALIDATE_SECRET", "");
    expect(((await readRevalidate(post("{}", ""))) as Response).status).toBe(401);
  });

  it("keeps every tag and path a purge names, however many", async () => {
    const tags = Array.from({ length: 300 }, (_, i) => `product:${i}`);
    const got = await readRevalidate(post(JSON.stringify({ tags, paths: ["a", "/b"], warm: ["c"], channels: [CH, "nope"] })));
    expect(got).toEqual({ tags, paths: ["/a", "/b"], warm: ["/c"], channels: [CH] });
  });

  it("refuses a body past its size, said or streamed, and one that isn't JSON", async () => {
    const big = JSON.stringify({ tags: ["x".repeat(2 << 20)] });
    expect(((await readRevalidate(post(big))) as Response).status).toBe(413);
    expect(((await readRevalidate(post("{}", "s3cret", { "content-length": String(2 << 20) }))) as Response).status).toBe(413);
    expect(((await readRevalidate(post("{"))) as Response).status).toBe(400);
  });
});

describe("warmURL", () => {
  it("stays on the storefront's own origin", () => {
    expect(warmURL("http://shop.test", "/cart")).toBe("http://shop.test/cart");
    expect(warmURL("http://shop.test", "cart")).toBe("http://shop.test/cart");
    expect(warmURL("http://shop.test", "//evil.example/x")).toBe("http://shop.test//evil.example/x");
    expect(warmURL("http://shop.test", "@evil.example/x")).toBe("http://shop.test/@evil.example/x");
  });
});
