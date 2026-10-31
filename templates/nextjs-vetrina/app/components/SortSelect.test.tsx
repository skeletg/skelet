import { fireEvent, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";

import { renderWithIntl } from "@/app/test/render";

const push = vi.fn();
vi.mock("@/app/components/LocaleLink", () => ({
  Link: ({ href, children, ...rest }: { href: string; children: React.ReactNode }) => <a href={href} {...rest}>{children}</a>,
  useRouter: () => ({ push }),
}));

const { SortSelect } = await import("./SortSelect");

const options = [
  { slug: "relevance", label: "Relevance", isIndexable: true },
  { slug: "price-asc", label: "Price: low to high", isIndexable: true },
  { slug: "newest", label: "Newest", isIndexable: false },
];

it("keeps the search term and filters when sorting /search?q=shirt, and starts again from page one", () => {
  renderWithIntl(
    <SortSelect
      options={options}
      currentSlug="relevance"
      basePath="/search"
      pathFilters={[{ slug: "color", values: ["red"] }]}
      currentQuery="q=shirt&brand=acme&page=3"
    />,
  );
  const href = screen.getByRole("link", { name: "Price: low to high" }).getAttribute("href")!;
  const url = new URL(href, "http://shop.test");
  expect(url.pathname).toMatch(/^\/search\/.*red.*\/price-asc$/);
  expect(url.searchParams.get("q")).toBe("shirt");
  expect(url.searchParams.get("brand")).toBe("acme");
  expect(url.searchParams.has("page")).toBe(false);

  // A non-crawlable order navigates by push - the same URL
  fireEvent.click(screen.getByRole("button", { name: "Newest" }));
  expect(new URL(push.mock.calls[0][0], "http://shop.test").searchParams.get("q")).toBe("shirt");

  // Back to the default order drops the sort segment but not the search
  expect(screen.getByRole("link", { name: "Relevance" }).getAttribute("href")).toMatch(/\?q=shirt&brand=acme$/);
});
