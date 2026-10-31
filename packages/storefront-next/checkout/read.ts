// Checkout reads - the cart's saved checkout, and what its delivery groups are offered. All server-side: the page
// re-renders from these after every step instead of the browser fetching options itself

import { cache } from "react";

import {
  CheckoutStateDocument,
  DeliveryPointsDocument,
  PaymentMethodsDocument,
  type CheckoutStateQuery,
  type DeliveryPointsQuery,
} from "./operations.gql";
import { gqlFetch } from "../graphql";

import type { Requirement } from "./asks";

export type CheckoutState = Omit<NonNullable<CheckoutStateQuery["cart"]>["checkout"], "requirements"> & {
  requirements: Requirement[];
};
export type CheckoutAddress = NonNullable<CheckoutState["shippingAddress"]>;
export type CheckoutDestination = NonNullable<CheckoutState["destination"]>;
export type DeliveryGroup = CheckoutState["deliveryGroups"][number];
export type DeliveryChoice = DeliveryGroup["choices"][number];
export type DeliveryPoint = DeliveryPointsQuery["deliveryPoints"][number];
export type PaymentMethodOption = {
  providerSlug: string;
  displayName: string;
  sortOrder: number;
  instructions: string;
  isOffline: boolean;
};

// Throws on a backend failure - the page can't render a checkout it can't read, and the error boundary says so
export const getCheckoutState = cache(async (cartId: string): Promise<CheckoutState | null> => {
  const res = await gqlFetch(CheckoutStateDocument, { id: cartId });
  return (res.data.cart?.checkout as CheckoutState | undefined) ?? null;
});

// This demo ships no gateway SDK, so it offers only methods with no third party behind them - a gateway method would
// place an order whose payment nobody can complete
export async function getPaymentMethods(countryCode: string, cartId: string): Promise<PaymentMethodOption[]> {
  try {
    const res = await gqlFetch(PaymentMethodsDocument, { countryCode, cartId });
    return res.data.paymentMethods.filter((m) => m.isOffline).sort((a, b) => a.sortOrder - b.sortOrder);
  } catch {
    return [];
  }
}

// What this checkout can pay with: eligible in the destination's country (or the billing address's, for a cart with
// nothing to ship), narrowed to an offer's own list. The page lists these and placeOrderAction rechecks against them
export async function checkoutPaymentMethods(
  checkout: CheckoutState,
  cartId: string,
  offerMethods: string[] = [],
): Promise<PaymentMethodOption[]> {
  const country = checkout.destination?.countryCode ?? checkout.billingAddress?.countryCode;
  if (!country) return [];
  const methods = await getPaymentMethods(country, cartId);
  return offerMethods.length ? methods.filter((m) => offerMethods.includes(m.providerSlug)) : methods;
}

// The stores or PO boxes a group's chosen delivery can go to, near the cart's destination - an unreachable
// source is reported separately from an empty result so the shopper can retry
export type DeliveryPointsResult = { points: DeliveryPoint[]; unavailable: boolean };

export async function getDeliveryPoints(cartId: string, groupKey: string, search?: string): Promise<DeliveryPointsResult> {
  try {
    const res = await gqlFetch(DeliveryPointsDocument, { cartId, groupKey, search: search || null });
    return { points: res.data.deliveryPoints, unavailable: false };
  } catch {
    return { points: [], unavailable: true };
  }
}
