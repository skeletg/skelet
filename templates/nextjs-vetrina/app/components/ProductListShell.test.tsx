import { render, screen } from "@testing-library/react";
import { createTranslator } from "next-intl";
import { beforeEach, expect, it, vi } from "vitest";

import messages from "@/messages/en.json";

vi.mock("react", async (original) => ({
  ...(await original<typeof import("react")>()),
  ViewTransition: ({ children }: { children: React.ReactNode }) => children,
}));
vi.mock("next-intl/server", () => ({
  getTranslations: async (namespace: string) => createTranslator({ locale: "en", messages, namespace: namespace as never }),
  getLocale: async () => "en",
}));
vi.mock("next/navigation", () => ({ permanentRedirect: vi.fn() }));
const browse = vi.hoisted(() => vi.fn());
vi.mock("@skelet/storefront-next/catalog/read", () => ({
  browseProducts: browse,
  browseProductsSlim: browse,
  productSortOptions: async () => [],
  synapseTrackedSlugs: async () => new Set<string>(),
}));
vi.mock("@skelet/storefront-next/config/read", () => ({
  getSeoConfig: async () => ({ facetCombinations: [], minFacetResults: 0 }),
  getBrowsePageSize: async () => 12,
  getPlpPaginationStyle: async () => "pagination",
}));
vi.mock("@skelet/storefront-next/channel/site", () => ({ getSiteUrl: async () => "https://shop.example" }));
vi.mock("@skelet/storefront-next/locale/path", () => ({ withLocalePrefix: async (p: string) => p }));
vi.mock("@skelet/storefront-next/locale/alternates", () => ({ canonicalPathMapper: async () => (p: string) => p }));
vi.mock("./FacetSidebar", () => ({ FacetSidebar: () => null }));
vi.mock("./SortSelect", () => ({ SortSelect: () => null }));
vi.mock("./Pagination", () => ({ Pagination: () => null }));
vi.mock("./LoadMore", () => ({ LoadMore: () => null }));
vi.mock("./JsonLd", () => ({ JsonLd: () => null }));
vi.mock("./LensTrack", () => ({ LensTrack: () => null }));
vi.mock("./ProductCard", () => ({ ProductCard: ({ product }: { product: { name: string } }) => <span>{product.name}</span> }));

const { ProductListShell } = await import("./ProductListShell");

const listing = (over: Record<string, unknown>) => ({
  data: { items: [{ id: "p1", name: "Linen shirt", sku: "LS-1", defaultPrice: 4900, defaultCurrencyCode: "EUR" }], totalCount: 1, facets: [], byMeaningOnly: false, aiOverview: false, ...over },
});

beforeEach(() => browse.mockReset());

const shell = async (over: Record<string, unknown>) => {
  browse.mockResolvedValue(listing(over));
  render(await ProductListShell({ query: "something for a beach wedding", searchParams: {}, basePath: "/search", overview: <p>The overview</p> }));
};

it("shows the overview only on a listing the backend offered one for", async () => {
  await shell({ aiOverview: true });
  expect(screen.getByText("The overview").closest(".listing-top")).not.toBeNull();
});

it("leaves the overview out when the backend didn't offer one", async () => {
  await shell({ aiOverview: false });
  expect(screen.queryByText("The overview")).toBeNull();
});

it("says when the results matched by meaning rather than by word, and only over results", async () => {
  await shell({ byMeaningOnly: true });
  expect(screen.getByText(/Nothing matched “something for a beach wedding” word for word/)).toBeInTheDocument();
});

it("says nothing about meaning over an empty listing", async () => {
  await shell({ byMeaningOnly: true, items: [], totalCount: 0 });
  expect(screen.queryByText(/word for word/)).toBeNull();
  expect(screen.getByText("No products match the current filters.")).toBeInTheDocument();
});
