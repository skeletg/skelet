import { beforeEach, expect, it, vi } from "vitest";

let jar = new Map<string, string>();
vi.mock("next/headers", () => ({
  headers: async () => new Headers(),
  cookies: async () => ({ get: (name: string) => (jar.has(name) ? { value: jar.get(name) } : undefined) }),
}));
vi.mock("@/app/lib/locale/read", () => ({
  getCurrentChannelId: async () => "ch-1",
  getCurrentLocale: async () => "en",
}));
vi.mock("server-only", () => ({}));

const { gqlFetch } = await import("./graphql");
const { ProductDocument } = await import("@/app/lib/catalog/operations.gql");
const { ACCESS_TOKEN_COOKIE } = await import("@/app/lib/auth/cookies");

const fetchMock = vi.fn();
vi.stubGlobal("fetch", fetchMock);

beforeEach(() => {
  jar = new Map();
  fetchMock.mockReset().mockResolvedValue(new Response(JSON.stringify({ data: { product: null } })));
});

const sent = () => fetchMock.mock.calls[0][1] as RequestInit & { headers: Record<string, string> };

it("sends a signed-in shopper's session with a public op and keeps it out of the shared cache", async () => {
  jar.set(ACCESS_TOKEN_COOKIE, "jwt-1");
  await gqlFetch(ProductDocument, { id: "p1" }, { cache: "force-cache", tags: ["product:p1"] });
  expect(sent().headers.Authorization).toBe("Bearer jwt-1");
  expect(sent().cache).toBe("no-store");
});

it("still caches the same op for an anonymous visitor", async () => {
  await gqlFetch(ProductDocument, { id: "p1" }, { cache: "force-cache", tags: ["product:p1"] });
  expect(sent().headers.Authorization).toBeUndefined();
  expect(sent().cache).toBe("force-cache");
});

it("treats a review voter token as a credential", async () => {
  await gqlFetch(ProductDocument, { id: "p1" }, { cache: "force-cache", extraHeaders: { "X-Review-Voter-Token": "v1" } });
  expect(sent().cache).toBe("no-store");
});
