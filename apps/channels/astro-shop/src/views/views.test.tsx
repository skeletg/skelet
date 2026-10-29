// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { CartView } from "./CartView";
import { CheckoutView, type CheckoutValues } from "./CheckoutView";
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

describe("CheckoutView", () => {
  const channel = { name: "Test Shop", defaultLocale: "en", publicUrl: "", currency: { code: "USD", symbol: "", symbolPosition: "prefix", decimals: 2 } };
  const totals = { subtotalCents: 3900, discountCents: 0, shippingCents: 0, taxCents: 0, totalCents: 3900 };
  const cart = {
    id: "c1", subtotal: 3900, totalDiscount: 0, lines,
    checkout: { email: "", paymentProviderSlug: null, destination: null, shippingAddress: null, deliveryGroups: [], requirements: [], totals },
  };
  const countries = [{ code: "RS", name: "Serbia" }, { code: "DE", name: "Germany" }];
  const values: CheckoutValues = {
    countryCode: "DE", postalCode: "", email: "ana@example.test", firstName: "Ana", lastName: "", street1: "", street2: "", city: "", state: "", phone: "", acceptTerms: false,
  };
  const choice = { optionId: "o1", optionName: "Door", kind: "ship_to_address", carrierSlug: "post", methodCode: "m1", methodName: "Standard", priceCents: 0, estimatedArrival: null, value: "o1|post|m1" };
  const base = { cart, channel, countries, groups: [], picks: new Map(), payments: [], values };

  it("asks where it's going first, and the summary has no shipping line yet", () => {
    const { container } = render(<CheckoutView {...base} dest={null} />);
    expect((screen.getByRole("combobox", { name: "Country" }) as HTMLSelectElement).value).toBe("DE");
    expect(screen.getByRole("button", { name: "Continue" })).toBeTruthy();
    expect(container.querySelector("[data-summary]")?.textContent).not.toContain("Shipping");
    expect(container.querySelector("#update-shipping")).toBeNull();
  });

  it("then takes delivery, details and payment in one form, keeping what was typed", () => {
    const payments = [{ providerSlug: "cod", displayName: "Cash on delivery", instructions: "Pay the courier." }];
    const { container } = render(
      <CheckoutView {...base} dest={{ countryCode: "RS", postalCode: "11000" }} groups={[{ key: "g1", selection: null, choices: [choice] }]} picks={new Map([["g1", "o1|post|m1"]])} payments={payments} pickedPayment="cod" error="Enter a valid email" />,
    );
    expect(screen.getByRole("alert").textContent).toBe("Enter a valid email");
    expect(container.textContent).toContain("To Serbia 11000");
    const delivery = screen.getByRole("radio", { name: /Standard/ }) as HTMLInputElement;
    expect([delivery.checked, delivery.value, delivery.dataset.autosubmit]).toEqual([true, "o1|post|m1", "update-shipping"]);
    expect((screen.getByRole("radio", { name: /Cash on delivery/ }) as HTMLInputElement).checked).toBe(true);
    expect((screen.getByRole("textbox", { name: "Email" }) as HTMLInputElement).value).toBe("ana@example.test");
    expect((screen.getByRole("button", { name: "Place order" }) as HTMLButtonElement).disabled).toBe(false);
    const update = container.querySelector<HTMLButtonElement>("#update-shipping");
    expect([update?.getAttribute("form"), update?.formNoValidate, update?.dataset.enhance]).toEqual(["checkout-details", true, "[data-summary]"]);
  });

  it("won't place an order nothing can deliver or pay for", () => {
    render(<CheckoutView {...base} dest={{ countryCode: "RS", postalCode: "11000" }} groups={[{ key: "g1", selection: null, choices: [] }]} />);
    expect(screen.getAllByRole("alert").map((a) => a.textContent)).toEqual(["These items can't be delivered there.", "No payment method is available for this country."]);
    expect((screen.getByRole("button", { name: "Place order" }) as HTMLButtonElement).disabled).toBe(true);
  });
});
