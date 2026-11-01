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
