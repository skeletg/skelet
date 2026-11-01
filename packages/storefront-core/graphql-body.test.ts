import { parse } from "graphql";
import { beforeEach, expect, it, vi } from "vitest";
import { useTestRequest } from "./testing";

useTestRequest({ headers: async () => new Headers(),
  cookies: async () => ({ get: () => undefined }) });
vi.mock("./locale/read", () => ({
  getCurrentChannelId: async () => "ch-1",
  getCurrentLocale: async () => "en",
}));

const { gqlFetch } = await import("./graphql");
const { ProductDocument } = await import("./catalog/operations.gql");

const fetchMock = vi.fn();
vi.stubGlobal("fetch", fetchMock);
beforeEach(() => {
  fetchMock.mockReset().mockResolvedValue(new Response(JSON.stringify({ data: { product: null } })));
});
const body = () => JSON.parse(fetchMock.mock.calls[0][1].body as string) as Record<string, unknown>;

// The backend's response cache allowlists by operationName - a body without it is never cached, however public the read
it("names the operation in the request body", async () => {
  await gqlFetch(ProductDocument, { id: "p1" });
  expect(body().operationName).toBe("Product");
  expect(body().query).toMatch(/^query Product\b/);
});

it("sends no operationName for an unnamed document", async () => {
  await gqlFetch(parse("{ __typename }"), {});
  expect(body()).not.toHaveProperty("operationName");
});

it("turns a transport failure or a bad status into the backend-unavailable error", async () => {
  const { BackendUnavailableError } = await import("./graphql");
  const quiet = vi.spyOn(console, "error").mockImplementation(() => {});
  fetchMock.mockRejectedValueOnce(new TypeError("fetch failed"));
  await expect(gqlFetch(ProductDocument, { id: "p1" })).rejects.toBeInstanceOf(BackendUnavailableError);
  fetchMock.mockResolvedValueOnce(new Response("down", { status: 502 }));
  const err = await gqlFetch(ProductDocument, { id: "p1" }).catch((e: unknown) => e);
  expect(err).toBeInstanceOf(BackendUnavailableError);
  expect((err as Error).message).not.toMatch(/Product|8080|http/);
  quiet.mockRestore();
});

it("sends no session with a read asked for as public", async () => {
  useTestRequest({ headers: async () => new Headers(), cookies: async () => ({ get: (n: string) => (n === "storefront_access_token" ? { value: "tok" } : undefined) }) });
  const { parse: parseDoc } = await import("graphql");
  const doc = parseDoc("query ShopHome { __typename }");
  fetchMock.mockImplementation(async () => new Response(JSON.stringify({ data: { __typename: "Query" } })));
  await gqlFetch(doc, {});
  expect((fetchMock.mock.calls[0][1].headers as Record<string, string>).Authorization).toBe("Bearer tok");
  await gqlFetch(doc, {}, { public: true });
  expect((fetchMock.mock.calls[1][1].headers as Record<string, string>).Authorization).toBeUndefined();
});
