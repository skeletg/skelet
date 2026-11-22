import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { CartLine } from "@skelet/storefront-core/cart/types";
import { renderWithIntl } from "@/app/test/render";

import { CartLineNotice } from "./CartLineNotice";

const line = (more: Partial<CartLine> = {}): CartLine => ({
  id: "l1", name: "Rain jacket", sku: "JACKET-M", unitPrice: 8500, quantity: 2, totalPrice: 17000, components: [], options: [], ...more,
});

// A price the store set for the shopper says so, with what the item usually costs and how many the price is for
describe("CartLineNotice", () => {
  it("says nothing on a line at its regular price", () => {
    const { container } = renderWithIntl(<CartLineNotice line={line()} currency="EUR" />);
    expect(container.textContent).toBe("");
  });

  it("shows a set price beside the regular one, and how many it is for", () => {
    renderWithIntl(<CartLineNotice line={line({ listUnitPrice: 10000, priceMaxQuantity: 2 })} currency="EUR" />);
    const note = screen.getByText(/A price set for you/);
    expect(note.textContent).toMatch(/usually .*100\.00/);
    expect(note.textContent).toContain("It is for up to 2.");
  });

  it("names no regular price when the line has none, or it is the same", () => {
    renderWithIntl(<CartLineNotice line={line({ listUnitPrice: 8500, priceMaxQuantity: 3 })} currency="EUR" />);
    const note = screen.getByText(/A price set for you/);
    expect(note.textContent).not.toMatch(/usually/);
    expect(note.textContent).toContain("It is for up to 3.");
  });
});
