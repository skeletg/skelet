import { screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import { renderWithIntl } from "@/app/test/render";

vi.mock("@/app/components/LocaleLink", () => ({
  Link: ({ href, children, ...rest }: { href: string; children: ReactNode }) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
  usePathname: () => "/elsewhere",
}));
vi.mock("@/app/components/AddToCartForm", () => ({ AddToCartForm: () => <button type="button">Add to cart</button> }));

const { ProductSpotlight } = await import("./ProductSpotlight");

const product = (over: Record<string, unknown>) =>
  ({ id: "p1", name: "Gift box", sku: "BOX", kind: "SIMPLE", url: "shop/box", defaultPrice: 5000, price: 5000, compareAtPrice: null, defaultCurrencyCode: "USD", inStock: true, mainImage: null, reviewAggregates: null, ...over }) as never;
const notes = { summary: "", highlights: [], goodFor: [], keepInMind: [] };

describe("ProductSpotlight", () => {
  it("quotes no price for a bundle priced by what's picked, the way its card doesn't", () => {
    renderWithIntl(<ProductSpotlight detail={{ product: product({ kind: "BUNDLE", price: 0, defaultPrice: 0, compareAtPrice: 4000 }), specs: [], notes }} />);
    expect(screen.queryByText("$0.00")).toBeNull();
    expect(screen.queryByText(/save/i)).toBeNull();
    expect(screen.getByText("See options")).toBeInTheDocument();
  });

  it("strikes through the usual price and badges the saving", () => {
    renderWithIntl(<ProductSpotlight detail={{ product: product({ price: 7500, compareAtPrice: 10000 }), specs: [], notes }} />);
    expect(screen.getByText("$75.00")).toBeInTheDocument();
    expect(screen.getByText("$100.00")).toBeInTheDocument();
    expect(screen.getByText(/25%/)).toBeInTheDocument();
  });

  it("links to the product once for a screen reader or a keyboard, by its name", () => {
    renderWithIntl(<ProductSpotlight detail={{ product: product({ mainImage: { url: "/m.jpg", width: 800, altText: "A box" } }), specs: [], notes }} />);
    const links = screen.getAllByRole("link");
    expect(links.map((l) => l.textContent)).toEqual(["Gift box", "View product"]);
    const image = document.querySelector("img")!;
    expect(image).toHaveAttribute("alt", "");
    expect(image.closest("a")).toHaveAttribute("tabindex", "-1");
    expect(image.closest("a")).toHaveAttribute("aria-hidden", "true");
  });

  it("draws repeated words from the model without tripping over them", () => {
    const error = vi.spyOn(console, "error").mockImplementation(() => {});
    renderWithIntl(
      <ProductSpotlight
        detail={{
          product: product({}),
          specs: [{ label: "Size", value: "M" }, { label: "Size", value: "L" }],
          notes: { summary: "Nice.", highlights: [{ title: "Sturdy", detail: "" }, { title: "Sturdy", detail: "really" }], goodFor: ["Gifts", "Gifts"], keepInMind: ["Heavy", "Heavy"] },
        }}
      />,
    );
    expect(screen.getAllByText("Sturdy")).toHaveLength(2);
    expect(error.mock.calls.filter((c) => String(c[0]).includes("same key"))).toEqual([]);
    error.mockRestore();
  });
});
