// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { CartView } from "./CartView";
import { NotFoundView } from "./NotFoundView";
import { OrderPlacedView } from "./OrderPlacedView";
import { ProductView } from "./ProductView";

afterEach(cleanup);

const usd = { code: "USD", symbol: "", decimals: 2 };
const lines = [
  { id: "l1", name: "Casual T-Shirt", sku: "TSHIRT", imageUrl: "https://cdn.test/shirt.jpg", unitPrice: 1500, quantity: 2, totalPrice: 3000 },
  { id: "l2", name: "Mug", sku: "MUG", imageUrl: null, unitPrice: 900, quantity: 1, totalPrice: 900 },
];

describe("NotFoundView", () => {
  it("says so and links back to the shop", () => {
    render(<NotFoundView />);
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("We couldn't find that page");
    expect(screen.getByRole("link", { name: "Back to the shop" }).getAttribute("href")).toBe("/");
  });
});

describe("OrderPlacedView", () => {
  it("echoes the order number where the e2e suite reads it", () => {
    const { container } = render(<OrderPlacedView number="SO-1001" />);
    expect(container.querySelector("[data-order-number]")?.textContent).toBe("SO-1001");
  });

  it("leaves the number out when there is none", () => {
    const { container } = render(<OrderPlacedView />);
    expect(container.querySelector("[data-order-number]")).toBeNull();
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("Thank you for your order");
  });
});

describe("CartView", () => {
  it("shows the empty state inside the swapped region", () => {
    const { container } = render(<CartView cart={null} currency={usd} />);
    expect(container.querySelector("[data-cart]")?.textContent).toContain("Your cart is empty");
    expect(screen.getByRole("link", { name: "Keep shopping" })).toBeTruthy();
  });

  it("lists each line with its update and remove forms, the flash and the totals", () => {
    const { container } = render(<CartView cart={{ id: "c1", subtotal: 3900, totalDiscount: 400, lines }} flash="Only 2 left" currency={usd} />);
    expect(screen.getByRole("alert").textContent).toBe("Only 2 left");
    expect(container.querySelectorAll("[data-line]")).toHaveLength(2);
    expect((screen.getByRole("spinbutton", { name: "Quantity for Mug" }) as HTMLInputElement).value).toBe("1");
    const forms = container.querySelectorAll<HTMLFormElement>("form[data-enhance='[data-cart]']");
    expect([...forms].map((f) => (f.elements.namedItem("intent") as HTMLInputElement).value)).toEqual(["update", "remove", "update", "remove"]);
    expect(container.textContent).toContain("−$4.00");
    expect(screen.getByRole("link", { name: "Checkout" }).getAttribute("href")).toBe("/checkout");
  });
});

describe("ProductView", () => {
  const shirt = {
    id: "p1", url: "casual-t-shirt", sku: "TSHIRT", name: "Casual T-Shirt", kind: "SIMPLE" as const, inStock: true, price: 1500,
    compareAtPrice: null, onSale: false, minOrderQty: 2, quantityStep: 1, maxOrderQty: null, metaTitle: null, metaDescription: null,
    description: { root: { children: [{ children: [{ text: "Soft cotton tee." }] }] } }, mainImage: null, children: [],
  };
  const sku = (c: HTMLElement) => c.querySelector<HTMLFormElement>("form[action='/cart']")?.elements.namedItem("sku") as HTMLInputElement;

  it("adds a simple product as itself, from its minimum quantity", () => {
    const { container } = render(<ProductView product={shirt} currency={usd} />);
    expect(sku(container).value).toBe("TSHIRT");
    expect((screen.getByRole("spinbutton", { name: "Quantity" }) as HTMLInputElement).value).toBe("2");
    expect(screen.getByRole("button", { name: "Add to cart" })).toBeTruthy();
    expect(screen.getByText("Soft cotton tee.")).toBeTruthy();
  });

  it("offers a configurable product's variants, picking the first in stock", () => {
    const hoodie = { ...shirt, kind: "CONFIGURABLE" as const, name: "Hoodie", children: [
      { id: "v1", sku: "H-S", name: "Hoodie / Small", inStock: false, price: 4900 },
      { id: "v2", sku: "H-L", name: "Hoodie / Large", inStock: true, price: 4900 },
    ] };
    render(<ProductView product={hoodie} currency={usd} />);
    expect((screen.getByRole("radio", { name: /Small/ }) as HTMLInputElement).disabled).toBe(true);
    expect((screen.getByRole("radio", { name: /Large/ }) as HTMLInputElement).checked).toBe(true);
  });

  it("can't be bought sold out, and has no form when this shop can't sell it", () => {
    const { rerender, container } = render(<ProductView product={{ ...shirt, inStock: false }} currency={usd} />);
    expect((screen.getByRole("button", { name: "Sold out" }) as HTMLButtonElement).disabled).toBe(true);
    rerender(<ProductView product={{ ...shirt, kind: "BUNDLE" as const }} currency={usd} />);
    expect(container.querySelector("form")).toBeNull();
  });
});
