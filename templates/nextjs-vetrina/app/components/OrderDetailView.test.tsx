import { screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { renderWithIntl } from "@/app/test/render";
import { OrderDetailView, type OrderDetailOrder } from "./OrderDetailView";

const address = { firstName: "Ana", lastName: "Doe", company: "", street1: "1 Main St", street2: "", city: "Springfield", state: "", postalCode: "12345", countryCode: "US", phone: "" };

// What this pins: a bundle line on the customer's order lists what's in the box, as the cart did before checkout
it("lists a bundle line's components under it", () => {
  const order = {
    id: "o1", number: "ORD-1", status: "PENDING", paymentStatus: "PAID", paymentProvider: "manual", paymentInstructions: null,
    subtotal: 3000, shippingAmount: 0, discountAmount: 0, taxAmount: 0, total: 3000, giftCardAmount: 0, storeCreditAmount: 0, amountDue: 0,
    currencyCode: "USD", customerNote: null, carrier: "", trackingNumber: "", trackingUrl: null, createdAt: "2026-10-30T10:00:00Z",
    billingAddress: address, shippingAddress: address,
    lines: [{
      id: "l1", name: "Gift box", sku: "BOX", unitPrice: 3000, quantity: 1, totalPrice: 3000, options: [],
      components: [{ id: "c1", sku: "MUG", name: "Mug", quantity: 2 }, { id: "c2", sku: "TEA", name: "Tea", quantity: 1 }],
    }],
  } as unknown as OrderDetailOrder;
  renderWithIntl(<OrderDetailView order={order} />);
  expect(screen.getByText("↳ Mug ×2")).toBeInTheDocument();
  expect(screen.getByText("↳ Tea ×1")).toBeInTheDocument();
});

// What this pins: tax that sits inside the prices is labelled as included, not as a charge on top
it("labels tax as included when the prices already held it", () => {
  const order = {
    id: "o2", number: "ORD-2", status: "PENDING", paymentStatus: "PAID", paymentProvider: "manual", paymentInstructions: null,
    subtotal: 1200, shippingAmount: 0, discountAmount: 0, taxAmount: 200, taxIncluded: true, total: 1200, giftCardAmount: 0, storeCreditAmount: 0, amountDue: 0,
    currencyCode: "EUR", customerNote: null, carrier: "", trackingNumber: "", trackingUrl: null, createdAt: "2026-11-08T10:00:00Z",
    billingAddress: address, shippingAddress: address, lines: [],
  } as unknown as OrderDetailOrder;
  renderWithIntl(<OrderDetailView order={order} />);
  expect(screen.getByText("Includes tax")).toBeInTheDocument();
  expect(screen.queryByText("Tax")).not.toBeInTheDocument();
});
