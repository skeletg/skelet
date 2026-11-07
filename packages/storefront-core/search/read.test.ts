import { beforeEach, expect, it, vi } from "vitest";

const { gqlFetch } = vi.hoisted(() => ({ gqlFetch: vi.fn() }));
vi.mock("../graphql", () => ({ gqlFetch }));
const { searchOverview } = await import("./read");
const { SearchOverviewDocument } = await import("./operations.gql");

beforeEach(() => gqlFetch.mockReset());

const product = { __typename: "SearchOverviewProduct", id: "p1", name: "Linen Shirt Dress", url: "shop/dresses/linen-shirt-dress" };

it("asks for the trimmed query under a hard deadline, and hands back just the text and products", async () => {
  gqlFetch.mockResolvedValue({ data: { searchOverview: { text: "Try [[1]].", products: [product] } } });
  expect(await searchOverview("  summer wedding ")).toEqual({ text: "Try [[1]].", products: [{ id: "p1", name: "Linen Shirt Dress", url: "shop/dresses/linen-shirt-dress" }] });
  const [doc, vars, opts] = gqlFetch.mock.calls[0];
  expect(doc).toBe(SearchOverviewDocument);
  expect(vars).toEqual({ query: "summer wedding" });
  expect(opts.signal).toBeInstanceOf(AbortSignal);
  expect(opts).not.toHaveProperty("cache");
});

it("shows nothing for no query, no overview, an empty one, a failure or a timeout", async () => {
  expect(await searchOverview("   ")).toBeNull();
  expect(gqlFetch).not.toHaveBeenCalled();
  gqlFetch
    .mockResolvedValueOnce({ data: { searchOverview: null } })
    .mockResolvedValueOnce({ data: { searchOverview: { text: "  ", products: [product] } } })
    .mockRejectedValueOnce(new Error("rate limited"))
    .mockRejectedValueOnce(new DOMException("timed out", "TimeoutError"));
  for (let i = 0; i < 4; i++) expect(await searchOverview("wedding")).toBeNull();
});
