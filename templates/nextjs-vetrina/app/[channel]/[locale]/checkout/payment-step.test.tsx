import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useEffect } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { CheckoutState } from "@skelet/storefront-next/checkout/read";
import { renderWithIntl } from "@/app/test/render";
import { __resetForTests, registerPaymentUI, type PaymentHandle, type PaymentUI } from "@/app/plugins/registry";
import type { SavedPaymentMethod } from "@skelet/storefront-next/payment-methods/actions";

const actions = vi.hoisted(() => ({
  place: vi.fn(),
  finalize: vi.fn(async () => {}),
}));
vi.mock("@skelet/storefront-next/checkout/actions", () => ({
  placeOrderAction: actions.place,
  finalizePlacedPaymentAction: actions.finalize,
  applyGiftCardAction: vi.fn(),
  removeGiftCardAction: vi.fn(),
  setStoreCreditAction: vi.fn(),
}));

const { PaymentStep } = await import("@/app/[channel]/[locale]/checkout/payment-step");

const checkout = {
  email: "ana@example.com",
  paymentProviderSlug: "card",
  giftCards: [],
  useStoreCredit: false,
  totals: { subtotalCents: 2000, discountCents: 0, shippingCents: 0, taxCents: 0, totalCents: 2000, giftCardCents: 0, storeCreditCents: 0, amountDueCents: 2000 },
} as unknown as CheckoutState;

function renderStep(savedCards: SavedPaymentMethod[] = []) {
  return renderWithIntl(
    <PaymentStep
      methods={[{ providerSlug: "card", displayName: "Card", sortOrder: 0, instructions: "", isOffline: false }]}
      checkout={checkout}
      note=""
      savedCards={savedCards}
      storeCreditBalance={0}
      signedIn={false}
      emailOptIn={false}
      consent="notice"
      currency="EUR"
    />,
  );
}

// A gateway plugin's fields, standing in for a real browser SDK - they register once loaded, the way the contract asks
function registerCard(handle: PaymentHandle | null, extra: Partial<PaymentUI> = {}) {
  registerPaymentUI({
    provider: "card",
    Fields: function CardFields({ register }) {
      useEffect(() => {
        if (!handle) return;
        register(handle);
        return () => register(null);
      }, [register]);
      return <p>card fields</p>;
    },
    ...extra,
  });
}

const placed = { orderId: "order-1", clientSecret: "pi_secret", email: "ana@example.com", cartId: "cart-1", offer: false };

afterEach(() => {
  __resetForTests();
  vi.clearAllMocks();
});

// What these pin: the gateway's token is made at Pay and travels with the order, a refused card keeps the shopper on
// the step, and an in-page challenge runs before the gateway is asked how it went
describe("paying through a gateway's fields", () => {
  it("tokenizes at Pay and places the order with the token", async () => {
    const tokenize = vi.fn(async () => "tok_123");
    registerCard({ tokenize });
    actions.place.mockResolvedValue({ status: "success" });
    renderStep();
    expect(screen.getByText("card fields")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /Pay/ }));
    await waitFor(() => expect(actions.place).toHaveBeenCalled());
    expect(tokenize).toHaveBeenCalledOnce();
    const sent = actions.place.mock.calls[0][1] as FormData;
    expect(sent.get("paymentToken")).toBe("tok_123");
    expect(sent.get("paymentUi")).toBe("on");
  });

  it("keeps the shopper on the step when the card is refused", async () => {
    registerCard({ tokenize: async () => Promise.reject(new Error("Your card number is incomplete.")) });
    renderStep();
    await userEvent.click(screen.getByRole("button", { name: /Pay/ }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Your card number is incomplete.");
    expect(actions.place).not.toHaveBeenCalled();
  });

  it("runs the challenge, then lets the gateway say how it went", async () => {
    const challenge = vi.fn(async () => {});
    registerCard({ tokenize: async () => "tok_123", challenge });
    actions.place.mockResolvedValue({ status: "success", payment: placed });
    renderStep();
    await userEvent.click(screen.getByRole("button", { name: /Pay/ }));
    await waitFor(() => expect(actions.finalize).toHaveBeenCalled());
    expect(challenge).toHaveBeenCalledWith("pi_secret");
    expect(actions.finalize).toHaveBeenCalledWith({ orderId: "order-1", email: "ana@example.com", cartId: "cart-1", offer: false });
  });

  it("holds Pay until the card fields have loaded", async () => {
    registerCard(null);
    renderStep();
    expect(await screen.findByText("card fields")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Placing/ })).toBeDisabled();
  });

  it("runs a saved card's challenge through the gateway's registration, with no fields mounted", async () => {
    const challenge = vi.fn(async () => {});
    registerCard({ tokenize: async () => "tok_123" }, { challenge });
    actions.place.mockResolvedValue({ status: "success", payment: placed });
    renderStep([{ id: "saved-1", providerSlug: "card", brand: "Visa", last4: "4242", expMonth: 12, expYear: 2039 } as SavedPaymentMethod]);
    await userEvent.click(screen.getByRole("radio", { name: /4242/ }));
    expect(screen.queryByText("card fields")).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /Pay/ }));
    await waitFor(() => expect(actions.finalize).toHaveBeenCalled());
    const sent = actions.place.mock.calls[0][1] as FormData;
    expect(sent.get("paymentUi")).toBe("on");
    expect(sent.get("paymentToken")).toBeNull();
    expect(challenge).toHaveBeenCalledWith("pi_secret");
  });

  it("offers the order page when finishing the payment fails", async () => {
    registerCard({ tokenize: async () => "tok_123", challenge: async () => {} });
    actions.place.mockResolvedValue({ status: "success", payment: placed });
    actions.finalize.mockRejectedValueOnce(new Error("network"));
    renderStep();
    await userEvent.click(screen.getByRole("button", { name: /Pay/ }));
    const link = await screen.findByRole("link", { name: "Open your order" });
    expect(link.getAttribute("href")).toContain("/orders/order-1?email=ana%40example.com&payment=incomplete");
  });

  it("places as before when no gateway UI is registered", async () => {
    actions.place.mockResolvedValue({ status: "success" });
    renderStep();
    await userEvent.click(screen.getByRole("button", { name: /Pay/ }));
    await waitFor(() => expect(actions.place).toHaveBeenCalled());
    const sent = actions.place.mock.calls[0][1] as FormData;
    expect(sent.get("paymentToken")).toBeNull();
    expect(sent.get("paymentUi")).toBeNull();
  });
});
