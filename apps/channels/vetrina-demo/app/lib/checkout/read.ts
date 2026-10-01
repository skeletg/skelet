// Checkout reads - the cart's saved checkout, and the options its destination is offered. All server-side: the page
// re-renders from these after every step instead of the browser fetching quotes itself

import { cache } from "react";

import {
  CheckoutStateDocument,
  PaymentMethodsDocument,
  PickupLocationsDocument,
  ShippingQuotesDocument,
  type CheckoutStateQuery,
} from "@/app/lib/checkout/operations.gql";
import { gqlFetch } from "@/app/lib/graphql";

export type CheckoutState = NonNullable<CheckoutStateQuery["cart"]>["checkout"];
export type CheckoutAddress = NonNullable<CheckoutState["shippingAddress"]>;
export type CheckoutDestination = NonNullable<CheckoutState["destination"]>;

export type ShippingQuote = {
  carrierSlug: string;
  carrierName: string;
  code: string;
  name: string;
  priceCents: number;
  // Business days, as the carrier quotes them - null when it gives no estimate
  estimatedDays?: number | null;
};

export type PaymentMethodOption = {
  providerSlug: string;
  displayName: string;
  sortOrder: number;
  instructions: string;
};

export type PickupLocation = {
  slug: string;
  name: string;
  line1: string;
  city: string;
  postalCode: string;
  countryCode: string;
  instructions: string;
};

// Throws on a backend failure - the page can't render a checkout it can't read, and the error boundary says so
export const getCheckoutState = cache(async (cartId: string): Promise<CheckoutState | null> => {
  const res = await gqlFetch(CheckoutStateDocument, { id: cartId });
  return res.data.cart?.checkout ?? null;
});

export async function getShippingQuotes(cartId: string, d: CheckoutDestination): Promise<ShippingQuote[]> {
  try {
    const res = await gqlFetch(ShippingQuotesDocument, {
      countryCode: d.countryCode,
      cartId,
      regionCode: d.regionCode || null,
      postalCode: d.postalCode || null,
    });
    return res.data.shippingQuotes;
  } catch {
    return [];
  }
}

export async function getPaymentMethods(countryCode: string): Promise<PaymentMethodOption[]> {
  try {
    const res = await gqlFetch(PaymentMethodsDocument, { countryCode });
    return res.data.paymentMethods.slice().sort((a, b) => a.sortOrder - b.sortOrder);
  } catch {
    return [];
  }
}

// Click-and-collect off or unreachable leaves pickup quotes without their store details, and delivery is unaffected
export async function getPickupLocations(): Promise<PickupLocation[]> {
  try {
    const res = await gqlFetch(PickupLocationsDocument);
    return res.data.pickupLocations;
  } catch {
    return [];
  }
}
