import { beforeEach, describe, expect, it, vi } from "vitest";

const browseProducts = vi.fn();
const getBrowsePageSize = vi.fn();
const getSeoConfig = vi.fn();
const evaluateFilterIndexability = vi.fn();

vi.mock("@/app/lib/catalog/read", () => ({ browseProducts: (...a: unknown[]) => browseProducts(...a) }));
vi.mock("@/app/lib/config/read", () => ({
  getBrowsePageSize: () => getBrowsePageSize(),
  getSeoConfig: () => getSeoConfig(),
}));
vi.mock("@/app/lib/filter-path", () => ({
  evaluateFilterIndexability: (...a: unknown[]) => evaluateFilterIndexability(...a),
}));

const { listingIndexability } = await import("@/app/lib/catalog/indexability");

const brand = [{ slug: "brand", values: ["atelier"] }];
const facets = [{ slug: "brand", values: [] }];

beforeEach(() => {
  vi.clearAllMocks();
  getBrowsePageSize.mockResolvedValue(24);
  getSeoConfig.mockResolvedValue({ facetCombinations: [["brand"]], minFacetResults: 5 });
  browseProducts.mockResolvedValue({ data: { facets, totalCount: 40 } });
  evaluateFilterIndexability.mockImplementation(() => ({ indexable: true, values: [] }));
});

describe("listingIndexability", () => {
  it("browses under the listing's base filter at the grid's page size, so it shares the grid's cached fetch", async () => {
    await listingIndexability({ categoryId: "cat-1" }, brand);
    expect(browseProducts).toHaveBeenCalledWith({
      filter: { categoryId: "cat-1", attributes: brand },
      limit: 24,
      offset: 0,
    });
  });

  it("sends null attributes for an unfiltered listing", async () => {
    await listingIndexability({ collectionId: "col-1" }, []);
    expect(browseProducts.mock.calls[0][0].filter).toEqual({ collectionId: "col-1", attributes: null });
  });

  it("browses the whole catalog for the products listing", async () => {
    await listingIndexability({}, brand);
    expect(browseProducts.mock.calls[0][0].filter).toEqual({ attributes: brand });
  });

  it("applies the facet rules to the browse result's facets", async () => {
    await listingIndexability({}, brand);
    expect(evaluateFilterIndexability).toHaveBeenCalledWith(brand, facets, { combinations: [["brand"]] });
  });

  it("noindexes a filtered page with fewer results than the thin-page minimum", async () => {
    browseProducts.mockResolvedValue({ data: { facets, totalCount: 3 } });
    expect((await listingIndexability({}, brand)).indexable).toBe(false);
  });

  it("keeps a sparse unfiltered listing indexable", async () => {
    browseProducts.mockResolvedValue({ data: { facets, totalCount: 3 } });
    expect((await listingIndexability({}, [])).indexable).toBe(true);
  });

  it("skips the thin-page guard when the minimum is off", async () => {
    getSeoConfig.mockResolvedValue({ facetCombinations: [], minFacetResults: 0 });
    browseProducts.mockResolvedValue({ data: { facets, totalCount: 1 } });
    expect((await listingIndexability({}, brand)).indexable).toBe(true);
  });

  it("never turns a facet-rule noindex back on", async () => {
    evaluateFilterIndexability.mockImplementation(() => ({ indexable: false, values: [] }));
    expect((await listingIndexability({}, brand)).indexable).toBe(false);
  });
});
