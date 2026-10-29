// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { CartView } from "./CartView";
import { NotFoundView } from "./NotFoundView";
import { OrderPlacedView } from "./OrderPlacedView";

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
